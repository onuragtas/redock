package email_server

import (
	"redock/platform/memory"
	"time"
)

type EmailDomain struct {
	memory.SoftDeleteEntity
	Domain          string     `json:"domain"`
	Enabled         bool       `json:"enabled"`
	Description     string     `json:"description"`
	MaxMailboxes    int        `json:"max_mailboxes"`
	MaxQuotaPerBox  int64      `json:"max_quota_per_box"`
	TotalQuota      int64      `json:"total_quota"`
	UsedQuota       int64      `json:"used_quota"`
	DNSConfigured   bool       `json:"dns_configured"`
	MXRecord        string     `json:"mx_record"`
	SPFRecord       string     `json:"spf_record"`
	DKIMSelector    string     `json:"dkim_selector"`
	DKIMPublicKey   string     `json:"dkim_public_key"`
	DKIMPrivateKey  string     `json:"dkim_private_key"`
	DMARCRecord     string     `json:"dmarc_record"`
	CatchAll        string     `json:"catch_all"`
	EnableSPAM      bool       `json:"enable_spam"`
	EnableVirus     bool       `json:"enable_virus"`
	SMTPOnly        bool       `json:"smtp_only"`
	LastSync        *time.Time `json:"last_sync"`
}

type EmailMailbox struct {
	memory.SoftDeleteEntity
	DomainID        uint       `json:"domain_id"`
	Username        string     `json:"username"`
	Email           string     `json:"email"`
	Password        string     `json:"password"`
	PlainPassword   string     `json:"plain_password,omitempty"`
	Name            string     `json:"name"`
	Quota           int64      `json:"quota"`
	UsedQuota       int64      `json:"used_quota"`
	MessageCount    int        `json:"message_count"`
	Enabled         bool       `json:"enabled"`
	ForwardTo       string     `json:"forward_to"`
	KeepCopy        bool       `json:"keep_copy"`
	AutoReply       bool       `json:"auto_reply"`
	AutoReplyMsg    string     `json:"auto_reply_msg"`
	IMAPEnabled     bool       `json:"imap_enabled"`
	POP3Enabled     bool       `json:"pop3_enabled"`
	SMTPEnabled     bool       `json:"smtp_enabled"`
	LastLogin       *time.Time `json:"last_login"`
	LastActivity    *time.Time `json:"last_activity"`
	LoginCount      int        `json:"login_count"`
}

type EmailAlias struct {
	memory.SoftDeleteEntity
	DomainID        uint       `json:"domain_id"`
	Alias           string     `json:"alias"`
	DestinationID   uint       `json:"destination_id"`
	Destination     string     `json:"destination"`
	Enabled         bool       `json:"enabled"`
}

type EmailFolder struct {
	memory.SoftDeleteEntity
	MailboxID       uint       `json:"mailbox_id"`
	Name            string     `json:"name"`
	Path            string     `json:"path"`
	ParentID        *uint      `json:"parent_id"`
	MessageCount    int        `json:"message_count"`
	UnreadCount     int        `json:"unread_count"`
	IsSystem        bool       `json:"is_system"`
	Icon            string     `json:"icon"`
	Color           string     `json:"color"`
}

type Email struct {
	memory.SoftDeleteEntity
	MailboxID       uint       `json:"mailbox_id"`
	FolderID        uint       `json:"folder_id"`
	MessageID       string     `json:"message_id"`
	UID             uint32     `json:"uid"`
	From            string     `json:"from"`
	To              string     `json:"to"`
	CC              string     `json:"cc"`
	BCC             string     `json:"bcc"`
	ReplyTo         string     `json:"reply_to"`
	Subject         string     `json:"subject"`
	Date            time.Time  `json:"date"`
	BodyPlain       string     `json:"body_plain"`
	BodyHTML        string     `json:"body_html"`
	HasAttachments  bool       `json:"has_attachments"`
	AttachmentCount int        `json:"attachment_count"`
	Size            int64      `json:"size"`
	Seen            bool       `json:"seen"`
	Answered        bool       `json:"answered"`
	Flagged         bool       `json:"flagged"`
	Deleted         bool       `json:"deleted"`
	Draft           bool       `json:"draft"`
	Recent          bool       `json:"recent"`
	InReplyTo       string     `json:"in_reply_to"`
	References      string     `json:"references"`
	ThreadID        string     `json:"thread_id"` // Konu zinciri: References'ın ilk Message-ID veya kendi Message-ID
	Labels          string     `json:"labels"`
	Priority        int        `json:"priority"`
	IsSpam          bool       `json:"is_spam"`
	SpamScore       float64    `json:"spam_score"`
	FilePath        string     `json:"file_path"`
}

type EmailAttachment struct {
	memory.SoftDeleteEntity
	EmailID         uint       `json:"email_id"`
	Filename        string     `json:"filename"`
	ContentType     string     `json:"content_type"`
	Size            int64      `json:"size"`
	ContentID       string     `json:"content_id"`
	IsInline        bool       `json:"is_inline"`
	FilePath        string     `json:"file_path"`
}

type EmailFilter struct {
	memory.SoftDeleteEntity
	MailboxID       uint       `json:"mailbox_id"`
	Name            string     `json:"name"`
	Priority        int        `json:"priority"`
	Enabled         bool       `json:"enabled"`
	Conditions      string     `json:"conditions"`
	MatchAll        bool       `json:"match_all"`
	Actions         string     `json:"actions"`
}

type EmailStats struct {
	MailboxID       uint       `json:"mailbox_id"`
	Date            time.Time  `json:"date"`
	SentCount       int        `json:"sent_count"`
	ReceivedCount   int        `json:"received_count"`
	SpamCount       int        `json:"spam_count"`
	VirusCount      int        `json:"virus_count"`
	SentBytes       int64      `json:"sent_bytes"`
	ReceivedBytes   int64      `json:"received_bytes"`
	TopSenders      string     `json:"top_senders"`
	TopRecipients   string     `json:"top_recipients"`
}

type EmailLog struct {
	memory.SoftDeleteEntity
	MailboxID       uint       `json:"mailbox_id"`
	Type            string     `json:"type"`
	From            string     `json:"from"`
	To              string     `json:"to"`
	Subject         string     `json:"subject"`
	Status          string     `json:"status"`
	StatusMessage   string     `json:"status_message"`
	Size            int64      `json:"size"`
	Timestamp       time.Time  `json:"timestamp"`
	SMTPCode        int        `json:"smtp_code"`
	RemoteIP        string     `json:"remote_ip"`
	RemoteHost      string     `json:"remote_host"`
	QueueID         string     `json:"queue_id"`
}

type EmailServerConfig struct {
	memory.SoftDeleteEntity
	Name            string     `json:"name"`
	Hostname        string     `json:"hostname"`
	IPAddress       string     `json:"ip_address"`
	SMTPPort        int        `json:"smtp_port"`
	SMTPSPort       int        `json:"smtps_port"`
	SubmissionPort  int        `json:"submission_port"`
	IMAPPort        int        `json:"imap_port"`
	IMAPsPort       int        `json:"imaps_port"`
	POP3Port        int        `json:"pop3_port"`
	POP3sPort       int        `json:"pop3s_port"`
	ContainerID     string     `json:"container_id"`
	ContainerName   string     `json:"container_name"`
	ImageName       string     `json:"image_name"`
	IsRunning       bool       `json:"is_running"`
	SSLEnabled      bool       `json:"ssl_enabled"`
	SSLCertPath     string     `json:"ssl_cert_path"`
	SSLKeyPath      string     `json:"ssl_key_path"`
	MaxMessageSize  int64      `json:"max_message_size"`
	MaxRecipients   int        `json:"max_recipients"`
	RateLimit       int        `json:"rate_limit"`
	SPAMEnabled     bool       `json:"spam_enabled"`
	VirusEnabled    bool       `json:"virus_enabled"`
	DKIMEnabled     bool       `json:"dkim_enabled"`
	DataPath        string     `json:"data_path"`
	ConfigPath      string     `json:"config_path"`
	LogPath         string     `json:"log_path"`
	LastStarted     *time.Time `json:"last_started"`
	LastStopped     *time.Time `json:"last_stopped"`
}
