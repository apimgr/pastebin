package notify

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// builtRequest is the wire form of a single webhook POST before signing headers
// are attached.
type builtRequest struct {
	method      string
	url         string
	contentType string
	body        []byte
}

// plainMessage renders a Message as the human-readable single-string form used
// by chat transports (Telegram, Discord, Slack, Mattermost, Pushover, Gotify).
func plainMessage(msg Message) string {
	var b strings.Builder
	if msg.Severity != "" {
		b.WriteString("[")
		b.WriteString(strings.ToUpper(msg.Severity))
		b.WriteString("] ")
	}
	b.WriteString(msg.Subject)
	if msg.Body != "" {
		b.WriteString("\n\n")
		b.WriteString(msg.Body)
	}
	return b.String()
}

// genericPayload is the JSON body sent to the generic transport (and any
// unrecognised transport name) per PART 12.
type genericPayload struct {
	Role           string `json:"role"`
	Event          string `json:"event"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	Severity       string `json:"severity"`
	Timestamp      int64  `json:"timestamp"`
	ProjectName    string `json:"project_name"`
	ProjectVersion string `json:"project_version"`
	AppURL         string `json:"app_url"`
	TrackingID     string `json:"tracking_id,omitempty"`
}

// buildRequest converts a Message into the transport-specific HTTP request for
// the given destination URL. Unknown transports fall through to the generic
// JSON adapter (PART 12: "any key under webhooks is treated as a transport
// name, and the adapter for that name is invoked").
func buildRequest(transport string, msg Message, meta Meta, dest string) (builtRequest, error) {
	text := plainMessage(msg)
	switch transport {
	case "telegram":
		sep := "&"
		if !strings.Contains(dest, "?") {
			sep = "?"
		}
		return builtRequest{
			method: "POST",
			url:    dest + sep + "text=" + url.QueryEscape(text),
		}, nil

	case "discord":
		body, err := json.Marshal(map[string]string{
			"content":  text,
			"username": meta.ProjectName,
		})
		if err != nil {
			return builtRequest{}, err
		}
		return builtRequest{method: "POST", url: dest, contentType: "application/json", body: body}, nil

	case "slack", "mattermost":
		body, err := json.Marshal(map[string]string{"text": text})
		if err != nil {
			return builtRequest{}, err
		}
		return builtRequest{method: "POST", url: dest, contentType: "application/json", body: body}, nil

	case "pushover":
		form := url.Values{}
		form.Set("title", msg.Subject)
		form.Set("message", messageOrSubject(msg))
		form.Set("priority", strconv.Itoa(pushoverPriority(msg.Severity)))
		return builtRequest{
			method:      "POST",
			url:         dest,
			contentType: "application/x-www-form-urlencoded",
			body:        []byte(form.Encode()),
		}, nil

	case "gotify":
		body, err := json.Marshal(map[string]any{
			"title":    msg.Subject,
			"message":  messageOrSubject(msg),
			"priority": gotifyPriority(msg.Severity),
		})
		if err != nil {
			return builtRequest{}, err
		}
		return builtRequest{method: "POST", url: dest, contentType: "application/json", body: body}, nil

	default:
		// generic and any unrecognised transport name.
		body, err := json.Marshal(genericPayload{
			Role:           msg.Role,
			Event:          msg.Event,
			Subject:        msg.Subject,
			Body:           msg.Body,
			Severity:       msg.Severity,
			Timestamp:      msg.Timestamp.Unix(),
			ProjectName:    meta.ProjectName,
			ProjectVersion: meta.ProjectVersion,
			AppURL:         meta.AppURL,
			TrackingID:     msg.TrackingID,
		})
		if err != nil {
			return builtRequest{}, err
		}
		return builtRequest{method: "POST", url: dest, contentType: "application/json", body: body}, nil
	}
}

// messageOrSubject returns the body when present, else the subject — used by
// transports that carry title and message separately.
func messageOrSubject(msg Message) string {
	if strings.TrimSpace(msg.Body) != "" {
		return msg.Body
	}
	return msg.Subject
}

// pushoverPriority maps a severity to the Pushover priority scale (-2..2).
func pushoverPriority(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 1
	case "warning":
		return 0
	default:
		return -1
	}
}

// gotifyPriority maps a severity to the Gotify priority scale (0..10).
func gotifyPriority(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 8
	case "warning":
		return 5
	default:
		return 2
	}
}
