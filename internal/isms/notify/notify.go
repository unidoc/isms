package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Config holds platform-level notification settings.
type Config struct {
	BaseURL string // e.g. https://isms.unidoc.io — used for links in notifications
}

// OrgChannels holds per-org notification channel credentials (from Postgres org_settings).
type OrgChannels struct {
	SlackWebhook string
	MatrixRoomID string
	MatrixToken  string
	MatrixServer string
}

// Notifier sends activity notifications to configured channels.
type Notifier struct {
	config Config
	client *http.Client
}

// safeHTTPClient returns an HTTP client configured to prevent SSRF attacks
// by blocking requests to private, loopback, and link-local IP addresses.
func safeHTTPClient() *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.LookupIP(host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
					return nil, errors.New("requests to private/internal IP addresses are not allowed")
				}
			}
			return net.Dial(network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
}

// New creates a new Notifier.
func New(cfg Config) *Notifier {
	return &Notifier{
		config: cfg,
		client: safeHTTPClient(),
	}
}

// Enabled always returns true — channels are checked per-org at send time.
func (n *Notifier) Enabled() bool {
	return true
}

// Event holds context for a notification.
type Event struct {
	Actor    string
	Action   string
	Detail   string
	Body     string // comment/task body (truncated)
	Link     string // relative URL path, e.g. /documents/ISO27001-4.1
	BaseURL  string // e.g. https://isms.unidoc.io
	Channels OrgChannels // per-org credentials
}

// Send dispatches a notification to the org's configured channels.
func (n *Notifier) Send(e Event) {
	if e.BaseURL == "" {
		e.BaseURL = n.config.BaseURL
	}
	icon := actionIcon(e.Action)
	if e.Channels.SlackWebhook != "" {
		go n.sendSlack(icon, e)
	}
	if e.Channels.MatrixRoomID != "" && e.Channels.MatrixToken != "" {
		go n.sendMatrix(icon, e)
	}
}

// SendSimple sends a simple notification.
func (n *Notifier) SendSimple(e Event) {
	n.Send(e)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (e Event) fullURL() string {
	if e.BaseURL == "" || e.Link == "" {
		return ""
	}
	return strings.TrimRight(e.BaseURL, "/") + "/" + strings.TrimLeft(e.Link, "/")
}

// --- Slack ---

type slackBlock struct {
	Type string      `json:"type"`
	Text interface{} `json:"text,omitempty"`
}
type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (n *Notifier) sendSlack(icon string, e Event) {
	headline := fmt.Sprintf("%s *%s* %s", icon, e.Actor, e.Detail)
	if url := e.fullURL(); url != "" {
		headline += fmt.Sprintf(" — <%s|View>", url)
	}
	blocks := []slackBlock{
		{Type: "section", Text: &slackText{Type: "mrkdwn", Text: headline}},
	}
	if e.Body != "" {
		blocks = append(blocks, slackBlock{
			Type: "context",
			Text: &slackText{Type: "mrkdwn", Text: "> " + truncate(e.Body, 300)},
		})
	}

	payload := map[string]interface{}{
		"text":   headline, // fallback for notifications
		"blocks": blocks,
	}
	body, _ := json.Marshal(payload)
	resp, err := n.client.Post(e.Channels.SlackWebhook, "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
}

// --- Matrix ---

func (n *Notifier) sendMatrix(icon string, e Event) {
	server := e.Channels.MatrixServer
	if server == "" {
		server = "https://matrix.org"
	}

	headline := fmt.Sprintf("%s <b>%s</b> %s", icon, e.Actor, e.Detail)
	plain := fmt.Sprintf("%s %s %s", icon, e.Actor, e.Detail)
	if url := e.fullURL(); url != "" {
		headline += fmt.Sprintf(` — <a href="%s">View</a>`, url)
		plain += " — " + url
	}
	if e.Body != "" {
		headline += "<br><blockquote>" + truncate(e.Body, 300) + "</blockquote>"
		plain += "\n> " + truncate(e.Body, 300)
	}

	txnID := fmt.Sprintf("isms-%d", time.Now().UnixNano())
	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		strings.TrimRight(server, "/"), e.Channels.MatrixRoomID, txnID)

	payload := map[string]string{
		"msgtype":        "m.text",
		"body":           plain,
		"format":         "org.matrix.custom.html",
		"formatted_body": headline,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+e.Channels.MatrixToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// --- Icons ---

func actionIcon(action string) string {
	switch {
	case strings.Contains(action, "approved"):
		return "\u2705"
	case strings.Contains(action, "merged"):
		return "\U0001F7E3"
	case strings.Contains(action, "changes"):
		return "\U0001F7E0"
	case strings.Contains(action, "comment"):
		return "\U0001F4AC"
	case strings.Contains(action, "review"):
		return "\U0001F4CB"
	case strings.Contains(action, "incident"):
		return "\U0001F6A8"
	case strings.Contains(action, "risk"):
		return "\u26A0\uFE0F"
	case strings.Contains(action, "task"):
		return "\u2611\uFE0F"
	case strings.Contains(action, "suggestion"):
		return "\U0001F4A1"
	default:
		return "\U0001F514"
	}
}
