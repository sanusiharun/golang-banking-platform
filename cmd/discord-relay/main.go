// discord-relay — receives Alertmanager webhook payloads and forwards them
// to a Discord channel as formatted embeds.
//
// Environment variables:
//
//	DISCORD_WEBHOOK_URL  — Discord webhook URL (required)
//	PORT                 — HTTP listen port (default: 9094)
//
// Alertmanager points its webhook_configs.url here:
//
//	http://discord-relay:9094/alert
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// ── Alertmanager webhook payload ──────────────────────────────────────────────

type amPayload struct {
	Status string  `json:"status"` // "firing" | "resolved"
	Alerts []alert `json:"alerts"`
}

type alert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
}

// ── Discord webhook payload ───────────────────────────────────────────────────

type discordPayload struct {
	Content string  `json:"content,omitempty"`
	Embeds  []embed `json:"embeds"`
}

type embed struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Color       int     `json:"color"` // decimal RGB
	Fields      []field `json:"fields,omitempty"`
	Footer      footer  `json:"footer,omitempty"`
}

type field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type footer struct {
	Text string `json:"text"`
}

// ── Colors ────────────────────────────────────────────────────────────────────

const (
	colorRed    = 0xED4245 // firing critical
	colorOrange = 0xFEE75C // firing warning
	colorGreen  = 0x57F287 // resolved
)

func main() {
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		log.Fatal("DISCORD_WEBHOOK_URL is not set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9094"
	}

	http.HandleFunc("/alert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		var payload amPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("failed to parse Alertmanager payload: %v", err)
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}

		dp := buildDiscordPayload(payload)
		if err := sendToDiscord(webhookURL, dp); err != nil {
			log.Printf("failed to send to Discord: %v", err)
			http.Error(w, "discord error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("discord-relay listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func buildDiscordPayload(payload amPayload) discordPayload {
	var embeds []embed

	for _, a := range payload.Alerts {
		severity := a.Labels["severity"]
		service := a.Labels["service"]
		alertName := a.Labels["alertname"]
		summary := a.Annotations["summary"]
		description := a.Annotations["description"]

		color := colorOrange
		icon := "⚠️"
		if a.Status == "resolved" {
			color = colorGreen
			icon = "✅"
		} else if severity == "critical" {
			color = colorRed
			icon = "🔴"
		}

		title := fmt.Sprintf("%s %s", icon, alertName)
		if a.Status == "resolved" {
			title = fmt.Sprintf("✅ RESOLVED: %s", alertName)
		}

		desc := summary
		if description != "" {
			desc = fmt.Sprintf("**%s**\n%s", summary, truncate(description, 300))
		}

		fields := []field{}
		if service != "" {
			fields = append(fields, field{Name: "Service", Value: service, Inline: true})
		}
		if severity != "" {
			fields = append(fields, field{Name: "Severity", Value: strings.ToUpper(severity), Inline: true})
		}
		fields = append(fields, field{
			Name:   "Started",
			Value:  a.StartsAt.Format("2006-01-02 15:04:05 UTC"),
			Inline: false,
		})

		embeds = append(embeds, embed{
			Title:       title,
			Description: desc,
			Color:       color,
			Fields:      fields,
			Footer:      footer{Text: "Banking Platform Alertmanager"},
		})
	}

	content := ""
	for _, a := range payload.Alerts {
		if a.Labels["severity"] == "critical" && a.Status == "firing" {
			content = "@here 🚨 Critical alert firing!"
			break
		}
	}

	return discordPayload{Content: content, Embeds: embeds}
}

func sendToDiscord(webhookURL string, payload discordPayload) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
