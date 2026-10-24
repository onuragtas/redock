package controllers

import (
	"errors"
	"strconv"
	"time"

	"redock/app/models"
	"redock/pkg/utils"
	"redock/platform/database"
	"redock/platform/memory"
	"redock/rtc_server"

	"github.com/gofiber/fiber/v2"
)

func rtcManager(c *fiber.Ctx) (*rtc_server.Manager, error) {
	m := rtc_server.GetManager()
	if m == nil {
		return nil, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "msg": "WebRTC server not initialized",
		})
	}
	return m, nil
}

func rtcResult(c *fiber.Ctx, data any, err error) error {
	if err != nil {
		status := fiber.StatusBadRequest
		var fe *fiber.Error
		if errors.As(err, &fe) {
			status = fe.Code // e.g. 404 for an unknown room
		}
		return c.Status(status).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": false, "msg": nil, "data": data})
}

// RTCStatus reports whether the SFU runs and its active rooms.
func RTCStatus(c *fiber.Ctx) error {
	m, err := rtcManager(c)
	if m == nil {
		return err
	}
	return rtcResult(c, m.Status(), nil)
}

// RTCGetConfig returns the SFU configuration (without the API secret).
func RTCGetConfig(c *fiber.Ctx) error {
	m, err := rtcManager(c)
	if m == nil {
		return err
	}
	cfg, cerr := m.Config()
	return rtcResult(c, cfg, cerr)
}

// RTCUpdateConfig saves the configuration and applies it (restart / route).
func RTCUpdateConfig(c *fiber.Ctx) error {
	m, err := rtcManager(c)
	if m == nil {
		return err
	}
	var in rtc_server.ConfigInput
	if err := c.BodyParser(&in); err != nil {
		return rtcResult(c, nil, err)
	}
	if err := m.UpdateConfig(in); err != nil {
		return rtcResult(c, nil, err)
	}
	return rtcResult(c, m.Status(), nil)
}

// RTCStart starts the SFU without changing the saved configuration.
func RTCStart(c *fiber.Ctx) error {
	m, err := rtcManager(c)
	if m == nil {
		return err
	}
	return rtcResult(c, m.Status(), m.Start())
}

// RTCStop stops the SFU (all calls end).
func RTCStop(c *fiber.Ctx) error {
	m, err := rtcManager(c)
	if m == nil {
		return err
	}
	return rtcResult(c, m.Status(), m.Stop())
}

// rtcHandler resolves the manager and wraps the result envelope.
func rtcHandler(fn func(c *fiber.Ctx, m *rtc_server.Manager) (any, error)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		m, err := rtcManager(c)
		if m == nil {
			return err
		}
		data, ferr := fn(c, m)
		return rtcResult(c, data, ferr)
	}
}

type rtcInviteBody struct {
	Room     string  `json:"room"`
	Username string  `json:"username"`
	Role     string  `json:"role"`
	TTLHours float64 `json:"ttl_hours"`
}

func (b rtcInviteBody) ttl() time.Duration {
	return time.Duration(b.TTLHours * float64(time.Hour))
}

var (
	RTCListRooms = rtcHandler(func(_ *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		return m.Rooms(), nil
	})
	RTCSaveRoom = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		var in rtc_server.RoomInput
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		room, err := m.SaveRoom(in)
		if err == nil {
			m.Audit(rtcActor(c), "room-saved", room.Name, "", "")
		}
		return room, err
	})
	RTCDeleteRoom = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		var in struct {
			Name string `json:"name"`
		}
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		if err := m.DeleteRoom(in.Name); err != nil {
			return nil, err
		}
		m.Audit(rtcActor(c), "room-deleted", in.Name, "", "")
		return nil, nil
	})
	RTCInvite = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		var in rtcInviteBody
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		inv, err := m.IssueToken(in.Room, in.Username, in.Role, in.ttl())
		if err == nil {
			m.Audit(rtcActor(c), "invite", inv.Room, inv.Username, inv.Role+", valid until "+inv.ExpiresAt.Format(time.RFC3339))
		}
		return inv, err
	})

	RTCListAPIKeys = rtcHandler(func(_ *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		return m.APIKeys(), nil
	})
	RTCCreateAPIKey = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		var in struct {
			Name string `json:"name"`
		}
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		key, err := m.CreateAPIKey(in.Name)
		if err == nil {
			m.Audit(rtcActor(c), "apikey-created", "", key.Name, key.Prefix)
		}
		return key, err
	})
	RTCDeleteAPIKey = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		var in struct {
			ID uint `json:"id"`
		}
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		if err := m.DeleteAPIKey(in.ID); err != nil {
			return nil, err
		}
		m.Audit(rtcActor(c), "apikey-deleted", "", strconv.FormatUint(uint64(in.ID), 10), "")
		return nil, nil
	})
)

// RTCAPIKeyAuth authenticates external platforms by the X-API-Key header.
func RTCAPIKeyAuth(c *fiber.Ctx) error {
	m := rtc_server.GetManager()
	if m == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": true, "msg": "WebRTC server not initialized"})
	}
	name, ok := m.CheckAPIKey(c.Get("X-API-Key"))
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "msg": "invalid or missing X-API-Key"})
	}
	c.Locals(rtcActorKey, "api:"+name)
	return c.Next()
}

// External platform API (X-API-Key).
var (
	// RTCExtSaveRoom creates the room if needed (idempotent) or updates it.
	RTCExtSaveRoom = RTCSaveRoom
	// RTCExtToken issues a token (and join link) for a user of the platform.
	RTCExtToken = RTCInvite
	// RTCExtRoom reports a room's live state.
	RTCExtRoom = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		name := c.Params("name")
		for _, r := range m.Rooms() {
			if r.Name == name {
				return r, nil
			}
		}
		return nil, fiber.NewError(fiber.StatusNotFound, "room not found")
	})
)

const rtcActorKey = "rtcActor"

// rtcActor names who performs an action, for the audit log: the API key
// name for external platforms, otherwise the dashboard user's email.
func rtcActor(c *fiber.Ctx) string {
	if a, ok := c.Locals(rtcActorKey).(string); ok && a != "" {
		return a
	}
	claims, err := utils.ExtractTokenMetadata(c)
	if err != nil {
		return "unknown"
	}
	if db := database.GetMemoryDB(); db != nil {
		if u, err := memory.FindByID[*models.User](db, "users", uint(claims.UserID)); err == nil && u != nil && u.Email != "" {
			return u.Email
		}
	}
	return "user#" + strconv.Itoa(claims.UserID)
}

type rtcRoomBody struct {
	Room    string `json:"room"`
	ID      string `json:"id"`
	Reason  string `json:"reason"`
	Locked  bool   `json:"locked"`
	Message string `json:"message"`
}

func parseRoomBody(c *fiber.Ctx) (rtcRoomBody, error) {
	var in rtcRoomBody
	err := c.BodyParser(&in)
	return in, err
}

// Moderation from the dashboard (admin only).
var (
	RTCParticipants = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		return m.Participants(c.Params("name"))
	})
	RTCKick = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		in, err := parseRoomBody(c)
		if err != nil {
			return nil, err
		}
		return nil, m.Kick(rtcActor(c), in.Room, in.ID, in.Reason)
	})
	RTCEndSession = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		in, err := parseRoomBody(c)
		if err != nil {
			return nil, err
		}
		n, err := m.KickAll(rtcActor(c), in.Room, in.Reason)
		return fiber.Map{"removed": n}, err
	})
	RTCLock = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		in, err := parseRoomBody(c)
		if err != nil {
			return nil, err
		}
		return nil, m.SetLock(rtcActor(c), in.Room, in.Locked, in.Message)
	})
	RTCClearChat = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		in, err := parseRoomBody(c)
		if err != nil {
			return nil, err
		}
		return nil, m.ClearChat(rtcActor(c), in.Room)
	})
	RTCModeratorJoin = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		in, err := parseRoomBody(c)
		if err != nil {
			return nil, err
		}
		return m.ModeratorJoin(rtcActor(c), in.Room)
	})
	RTCAudit = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		limit := c.QueryInt("limit", 200)
		if limit <= 0 || limit > 2000 {
			limit = 200
		}
		return m.AuditLog(c.Query("room"), limit), nil
	})
)

// Recordings (admin only, except the signed download link itself).
var (
	RTCRecordings = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		recs, err := m.Recordings(c.Query("room"))
		return fiber.Map{"recordings": recs, "usage": m.RecordingUsage()}, err
	})
	RTCRecord = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		var in struct {
			Room  string `json:"room"`
			Start bool   `json:"start"`
		}
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		if in.Start {
			return nil, m.StartRecording(rtcActor(c), in.Room)
		}
		return nil, m.StopRecording(rtcActor(c), in.Room)
	})
	RTCRecordingLink = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		var in struct {
			Room string `json:"room"`
			Name string `json:"name"`
		}
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		link, err := m.DownloadLink(rtcActor(c), in.Room, in.Name)
		return fiber.Map{"url": link}, err
	})
	RTCDeleteRecording = rtcHandler(func(c *fiber.Ctx, m *rtc_server.Manager) (any, error) {
		var in struct {
			Room string `json:"room"`
			Name string `json:"name"`
		}
		if err := c.BodyParser(&in); err != nil {
			return nil, err
		}
		return nil, m.DeleteRecording(rtcActor(c), in.Room, in.Name)
	})
)

// RTCDownloadRecording serves a recording through a signed, short-lived
// link minted by RTCRecordingLink (no JWT: browsers can't add the header to
// a plain download).
func RTCDownloadRecording(c *fiber.Ctx) error {
	m := rtc_server.GetManager()
	if m == nil {
		return fiber.ErrServiceUnavailable
	}
	p, err := m.VerifyDownload(c.Query("room"), c.Query("name"), c.Query("exp"), c.Query("sig"))
	if err != nil {
		return c.Status(fiber.StatusForbidden).SendString(err.Error())
	}
	c.Set("Cache-Control", "no-store")
	return c.Download(p, c.Query("name"))
}
