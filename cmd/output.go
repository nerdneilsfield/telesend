package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nerdneilsfield/telesend/internal/dispatch"
	"github.com/nerdneilsfield/telesend/internal/telegram"
)

// stats summarizes a run.
type stats struct {
	Requests     int   `json:"requests"`
	Bytes        int64 `json:"bytes"`
	ElapsedMS    int64 `json:"elapsed_ms"`
	AvgRequestMS int64 `json:"avg_request_ms"`
	AvgSpeedBPS  int64 `json:"avg_speed_bps"`
}

type jsonResult struct {
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	MessageIDs []int  `json:"message_ids,omitempty"`
	Attempts   int    `json:"attempts"`
	TookMS     int64  `json:"took_ms"`
	Bytes      int64  `json:"bytes"`
	Skipped    bool   `json:"skipped,omitempty"`
	Error      string `json:"error,omitempty"`
}

type jsonReport struct {
	OK      bool         `json:"ok"`
	Error   string       `json:"error,omitempty"`
	Sent    int          `json:"sent"`
	Failed  int          `json:"failed"`
	Skipped int          `json:"skipped"`
	Results []jsonResult `json:"results"`
	Stats   stats        `json:"stats"`
}

// summary counts the results and computes the speed and latency averages.
type summary struct {
	sent, failed, skipped int
	stats                 stats
	avgRequest            time.Duration
	avgSpeed              float64 // Bytes per second of request time.
}

func summarize(results []dispatch.Result, elapsed time.Duration) summary {
	var s summary
	var took, uploadTook time.Duration
	for _, r := range results {
		s.stats.Requests += r.Attempts
		switch {
		case r.Skipped:
			s.skipped++
		case r.Err != nil:
			s.failed++
		default:
			s.sent++
			took += r.Took
			s.stats.Bytes += r.Bytes
			if r.Bytes > 0 {
				uploadTook += r.Took
			}
		}
	}
	if s.sent > 0 {
		s.avgRequest = took / time.Duration(s.sent)
	}
	if uploadTook > 0 {
		s.avgSpeed = float64(s.stats.Bytes) / uploadTook.Seconds()
	}
	s.stats.ElapsedMS = elapsed.Milliseconds()
	s.stats.AvgRequestMS = s.avgRequest.Milliseconds()
	s.stats.AvgSpeedBPS = int64(s.avgSpeed)
	return s
}

// writeJSON prints the machine-readable report for an agent.
func writeJSON(w io.Writer, results []dispatch.Result, s summary) error {
	rep := jsonReport{
		OK: s.failed == 0 && s.skipped == 0, Sent: s.sent, Failed: s.failed, Skipped: s.skipped,
		Results: make([]jsonResult, len(results)), Stats: s.stats,
	}
	for i, r := range results {
		rep.Results[i] = jsonResult{
			Name: r.Name, OK: r.Err == nil && !r.Skipped, MessageIDs: r.IDs, Attempts: r.Attempts,
			TookMS: r.Took.Milliseconds(), Bytes: r.Bytes, Skipped: r.Skipped,
		}
		if r.Err != nil {
			rep.Results[i].Error = r.Err.Error()
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// writeJSONError prints the report of a run that failed before it sent anything.
func writeJSONError(w io.Writer, err error) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonReport{Error: err.Error(), Results: []jsonResult{}})
}

// humanLine describes one sent job.
func humanLine(r dispatch.Result) string {
	line := fmt.Sprintf("sent %s  message %s  %s", r.Name, telegram.String(r.IDs), formatDuration(r.Took))
	if r.Bytes > 0 && r.Took > 0 {
		line += fmt.Sprintf("  %s at %s", formatBytes(float64(r.Bytes)), formatSpeed(float64(r.Bytes)/r.Took.Seconds()))
	}
	return line
}

// writeSummary prints the closing line of a run.
func writeSummary(w io.Writer, results []dispatch.Result, s summary) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Sent %d of %d in %s.", s.sent, len(results), formatDuration(time.Duration(s.stats.ElapsedMS)*time.Millisecond))
	if s.stats.Bytes > 0 && s.avgSpeed > 0 {
		fmt.Fprintf(&b, " %s at %s on average.", formatBytes(float64(s.stats.Bytes)), formatSpeed(s.avgSpeed))
	}
	if s.sent > 0 {
		fmt.Fprintf(&b, " Average request time %s.", formatDuration(s.avgRequest))
	}
	b.WriteByte('\n')
	_, err := io.WriteString(w, b.String())
	return err
}

// formatBytes uses decimal units: 1 MB is 1,000,000 bytes.
func formatBytes(n float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for n >= 1000 && i < len(units)-1 {
		n /= 1000
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f B", n)
	}
	return fmt.Sprintf("%.1f %s", n, units[i])
}

func formatSpeed(bps float64) string { return formatBytes(bps) + "/s" }

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
	m := int(d / time.Minute)
	return fmt.Sprintf("%d min %d s", m, int((d - time.Duration(m)*time.Minute).Seconds()))
}
