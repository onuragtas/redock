package email_server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-smtp"
)

// OutboundQueue delivers mail to the outside world: this is the part Postfix
// used to do. Messages live on disk so a restart never loses them, each with a
// small JSON sidecar tracking who still needs a copy and when to try again.
type OutboundQueue struct {
	manager *EmailManager
	dir     string

	mu      sync.Mutex
	running bool
	stop    chan struct{}
	wake    chan struct{}
	wg      sync.WaitGroup
}

// QueueItem is the sidecar for one queued message.
type QueueItem struct {
	ID         string    `json:"id"`
	From       string    `json:"from"`
	Recipients []string  `json:"recipients"`
	Domain     string    `json:"domain"` // sending domain, used for DKIM
	CreatedAt  time.Time `json:"created_at"`
	NextTry    time.Time `json:"next_try"`
	Attempts   int       `json:"attempts"`
	LastError  string    `json:"last_error,omitempty"`
	MailboxID  uint      `json:"mailbox_id,omitempty"`
	Subject    string    `json:"subject,omitempty"`
	Size       int64     `json:"size,omitempty"`
}

const (
	queueScanInterval  = 30 * time.Second
	queueDialTimeout   = 30 * time.Second
	defaultMaxAttempts = 12
	defaultRetryMin    = 15
)

func newOutboundQueue(m *EmailManager, dir string) *OutboundQueue {
	return &OutboundQueue{
		manager: m,
		dir:     dir,
		wake:    make(chan struct{}, 1),
	}
}

// Start begins the delivery worker.
func (q *OutboundQueue) Start() error {
	if err := os.MkdirAll(q.dir, 0700); err != nil {
		return err
	}

	q.mu.Lock()
	if q.running {
		q.mu.Unlock()
		return nil
	}
	q.running = true
	q.stop = make(chan struct{})
	stop := q.stop
	q.mu.Unlock()

	q.wg.Add(1)
	go q.run(stop)
	return nil
}

// Stop halts the worker, letting an in-flight delivery finish.
func (q *OutboundQueue) Stop() {
	q.mu.Lock()
	if !q.running {
		q.mu.Unlock()
		return
	}
	q.running = false
	close(q.stop)
	q.mu.Unlock()

	q.wg.Wait()
}

func (q *OutboundQueue) run(stop chan struct{}) {
	defer q.wg.Done()

	ticker := time.NewTicker(queueScanInterval)
	defer ticker.Stop()

	q.processDue()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			q.processDue()
		case <-q.wake:
			q.processDue()
		}
	}
}

// Enqueue stores a message for delivery and nudges the worker.
func (q *OutboundQueue) Enqueue(item *QueueItem, raw []byte) error {
	if err := os.MkdirAll(q.dir, 0700); err != nil {
		return err
	}
	if item.ID == "" {
		item.ID = newQueueID()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}
	if item.NextTry.IsZero() {
		item.NextTry = time.Now()
	}
	item.Size = int64(len(raw))

	if err := os.WriteFile(q.messagePath(item.ID), raw, 0600); err != nil {
		return err
	}
	if err := q.writeMeta(item); err != nil {
		os.Remove(q.messagePath(item.ID))
		return err
	}

	select {
	case q.wake <- struct{}{}:
	default:
	}
	return nil
}

func (q *OutboundQueue) messagePath(id string) string { return filepath.Join(q.dir, id+".eml") }
func (q *OutboundQueue) metaPath(id string) string    { return filepath.Join(q.dir, id+".json") }

func (q *OutboundQueue) writeMeta(item *QueueItem) error {
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(q.metaPath(item.ID), data, 0600)
}

func (q *OutboundQueue) remove(id string) {
	os.Remove(q.messagePath(id))
	os.Remove(q.metaPath(id))
}

// List returns the queue contents, oldest first, for the dashboard.
func (q *OutboundQueue) List() []QueueItem {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		return nil
	}

	items := make([]QueueItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(q.dir, entry.Name()))
		if err != nil {
			continue
		}
		var item QueueItem
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items
}

// Flush forces an immediate delivery attempt for every queued message.
func (q *OutboundQueue) Flush() int {
	items := q.List()
	for i := range items {
		items[i].NextTry = time.Now()
		_ = q.writeMeta(&items[i])
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return len(items)
}

// Delete drops a queued message without delivering it.
func (q *OutboundQueue) Delete(id string) error {
	if id == "" || strings.ContainsAny(id, "/\\.") {
		return fmt.Errorf("invalid queue id")
	}
	if _, err := os.Stat(q.metaPath(id)); err != nil {
		return err
	}
	q.remove(id)
	return nil
}

func (q *OutboundQueue) processDue() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("mail queue: delivery pass panicked: %v", r)
		}
	}()

	now := time.Now()
	for _, item := range q.List() {
		if item.NextTry.After(now) {
			continue
		}
		q.attempt(item)
	}
}

// attempt tries to deliver one queued message to its remaining recipients.
func (q *OutboundQueue) attempt(item QueueItem) {
	raw, err := os.ReadFile(q.messagePath(item.ID))
	if err != nil {
		log.Printf("mail queue: message %s unreadable, dropping: %v", item.ID, err)
		q.remove(item.ID)
		return
	}

	cfg := q.manager.nativeConfig()
	signed, err := q.manager.signMessage(item.Domain, raw)
	if err != nil {
		log.Printf("mail queue: %v", err)
		signed = raw
	}

	remaining := make([]string, 0, len(item.Recipients))
	var lastErr error
	permanent := false

	for domain, recipients := range groupByDomain(item.Recipients) {
		result, err := q.deliverToDomain(cfg, domain, item.From, recipients, signed)
		if err == nil {
			// The accepting reply usually carries the remote queue id, which is
			// what a receiving provider asks for when tracing a message.
			detail := fmt.Sprintf("accepted by %s: %s", result.Host, result.Accepted)
			if result.TLS {
				detail += " (over TLS)"
			}
			for _, rcpt := range result.Recipients {
				q.manager.logMailEvent(mailEvent{
					Direction:  "out",
					Status:     "sent",
					From:       item.From,
					To:         rcpt,
					Subject:    item.Subject,
					Size:       item.Size,
					MailboxID:  item.MailboxID,
					QueueID:    item.ID,
					Service:    "queue",
					SMTPCode:   result.Accepted.Code,
					RemoteHost: result.Host,
					Detail:     detail,
				})
			}
			continue
		}

		lastErr = err
		reply := replyOf(err)
		remoteHost := hostOfError(err)

		if isPermanentSMTPError(err) {
			permanent = true
			for _, rcpt := range recipients {
				q.manager.logMailEvent(mailEvent{
					Direction:  "out",
					Status:     "bounced",
					From:       item.From,
					To:         rcpt,
					Subject:    item.Subject,
					QueueID:    item.ID,
					Service:    "queue",
					SMTPCode:   reply.Code,
					RemoteHost: remoteHost,
					Detail:     err.Error(),
				})
			}
			continue
		}
		remaining = append(remaining, recipients...)
	}

	if len(remaining) == 0 {
		q.remove(item.ID)
		if permanent {
			q.bounce(item, signed, lastErr)
		}
		return
	}

	item.Recipients = remaining
	item.Attempts++
	if lastErr != nil {
		item.LastError = lastErr.Error()
	}

	maxAttempts := cfg.QueueMaxAttempts
	if item.Attempts >= maxAttempts {
		for _, rcpt := range remaining {
			q.manager.logMailEvent(mailEvent{
				Direction: "out",
				Status:    "bounced",
				From:      item.From,
				To:        rcpt,
				Subject:   item.Subject,
				QueueID:   item.ID,
				Detail:    fmt.Sprintf("giving up after %d attempts: %s", item.Attempts, item.LastError),
			})
		}
		q.remove(item.ID)
		q.bounce(item, signed, lastErr)
		return
	}

	// Linear-ish backoff: retryMinutes × attempts, capped at 4 hours.
	delay := time.Duration(cfg.QueueRetryMinutes) * time.Minute * time.Duration(item.Attempts)
	if delay > 4*time.Hour {
		delay = 4 * time.Hour
	}
	item.NextTry = time.Now().Add(delay)

	deferReply := replyOf(lastErr)
	deferHost := hostOfError(lastErr)
	for _, rcpt := range remaining {
		q.manager.logMailEvent(mailEvent{
			Direction:  "out",
			Status:     "deferred",
			From:       item.From,
			To:         rcpt,
			Subject:    item.Subject,
			QueueID:    item.ID,
			Service:    "queue",
			SMTPCode:   deferReply.Code,
			RemoteHost: deferHost,
			Detail:     fmt.Sprintf("attempt %d failed: %s (retry in %s)", item.Attempts, item.LastError, delay.Round(time.Minute)),
		})
	}
	_ = q.writeMeta(&item)
}

// deliverToDomain tries each of a domain's mail exchangers in turn.
func (q *OutboundQueue) deliverToDomain(cfg EmailServerConfig, domain, from string, recipients []string, raw []byte) (*DeliveryResult, error) {
	hosts, err := lookupMailHosts(domain)
	if err != nil {
		return nil, fmt.Errorf("MX lookup for %s failed: %w", domain, err)
	}

	helo := cfg.OutboundHELO
	if helo == "" {
		helo = cfg.Hostname
	}

	var lastErr error
	for _, host := range hosts {
		result, err := q.deliverToHost(host, helo, from, recipients, raw)
		if err == nil {
			return result, nil
		}
		lastErr = err
		// A final refusal from one exchanger will be repeated by the others.
		if isPermanentSMTPError(err) {
			return nil, err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no mail host reachable for %s", domain)
	}
	return nil, lastErr
}

// deliverToHost runs one SMTP transaction and hands back what the far side
// said, so the log can show the remote's own words rather than "failed".
func (q *OutboundQueue) deliverToHost(host, helo, from string, recipients []string, raw []byte) (*DeliveryResult, error) {
	return deliverSMTP(host, helo, from, recipients, raw)
}

// bounce delivers a delivery-status notification to the original sender when
// the sender is one of our own mailboxes; otherwise the failure only lands in
// the log (we do not relay bounces to strangers).
func (q *OutboundQueue) bounce(item QueueItem, raw []byte, cause error) {
	account := q.manager.LookupAccount(item.From)
	if account == nil {
		return
	}

	reason := "delivery failed"
	if cause != nil {
		reason = cause.Error()
	}

	body := strings.Builder{}
	body.WriteString("From: Mail Delivery System <postmaster@" + item.Domain + ">\r\n")
	body.WriteString("To: " + item.From + "\r\n")
	body.WriteString("Subject: Undelivered Mail Returned to Sender\r\n")
	body.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	body.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	body.WriteString("Your message could not be delivered.\r\n\r\n")
	body.WriteString("Recipients: " + strings.Join(item.Recipients, ", ") + "\r\n")
	body.WriteString("Attempts:   " + fmt.Sprint(item.Attempts) + "\r\n")
	body.WriteString("Reason:     " + reason + "\r\n\r\n")
	body.WriteString("Original subject: " + item.Subject + "\r\n")

	if _, err := q.manager.store().Deliver(account.Base, inboxName, []byte(body.String()), nil, time.Now()); err != nil {
		log.Printf("mail queue: could not deliver bounce to %s: %v", item.From, err)
	}
}

// groupByDomain buckets recipients by their domain so one connection serves
// every recipient at the same destination.
func groupByDomain(recipients []string) map[string][]string {
	grouped := make(map[string][]string)
	for _, rcpt := range recipients {
		_, domain := splitAddress(normalizeAddress(rcpt))
		if domain == "" {
			continue
		}
		grouped[domain] = append(grouped[domain], rcpt)
	}
	return grouped
}

// lookupMailHosts resolves a domain's mail exchangers in preference order,
// falling back to the domain itself when it publishes no MX (RFC 5321 §5.1).
func lookupMailHosts(domain string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resolver := net.DefaultResolver
	records, err := resolver.LookupMX(ctx, domain)
	if err == nil && len(records) > 0 {
		sort.SliceStable(records, func(i, j int) bool { return records[i].Pref < records[j].Pref })
		hosts := make([]string, 0, len(records))
		for _, mx := range records {
			hosts = append(hosts, strings.TrimSuffix(mx.Host, "."))
		}
		return hosts, nil
	}

	if addrs, aErr := resolver.LookupHost(ctx, domain); aErr == nil && len(addrs) > 0 {
		return []string{domain}, nil
	}
	if err == nil {
		err = fmt.Errorf("no MX or A record")
	}
	return nil, err
}

// isPermanentSMTPError reports whether a failure is final (5xx), meaning
// retrying would only waste time. Both client stacks in use are covered:
// net/smtp surfaces *textproto.Error, go-smtp surfaces *smtp.SMTPError.
func isPermanentSMTPError(err error) bool {
	if err == nil {
		return false
	}

	if reply := replyOf(err); reply.Code != 0 {
		return reply.Permanent()
	}

	var protoErr *textproto.Error
	if errorsAs(err, &protoErr) {
		return protoErr.Code >= 500 && protoErr.Code < 600
	}

	var smtpErr *smtp.SMTPError
	if errorsAs(err, &smtpErr) {
		return smtpErr.Code >= 500 && smtpErr.Code < 600
	}
	return false
}

func newQueueID() string {
	return fmt.Sprintf("%d%04d", time.Now().UnixNano(), os.Getpid()%10000)
}
