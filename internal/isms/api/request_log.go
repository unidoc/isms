package api

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// requestLogLine is one access-log line. Its field names and order reproduce
// the line echo's deprecated middleware.Logger wrote by default, so anything
// that parses the server's request log keeps working across the swap.
type requestLogLine struct {
	Time         string `json:"time"`
	ID           string `json:"id"`
	RemoteIP     string `json:"remote_ip"`
	Host         string `json:"host"`
	Method       string `json:"method"`
	URI          string `json:"uri"`
	UserAgent    string `json:"user_agent"`
	Status       int    `json:"status"`
	Error        string `json:"error"`
	Latency      int64  `json:"latency"`
	LatencyHuman string `json:"latency_human"`
	BytesIn      int64  `json:"bytes_in"`
	BytesOut     int64  `json:"bytes_out"`
}

// requestLogger replaces echo's deprecated middleware.Logger with
// RequestLoggerWithConfig, writing the same JSON line to the same place (the
// echo logger's output).
//
// HandleError runs the global error handler before the line is built, as
// Logger did, so the logged status is the one the client received. Unlike
// Logger, RequestLogger then returns the already-handled error up the chain;
// echo's default error handler ignores it because the response is committed.
func requestLogger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		HandleError:      true,
		LogRequestID:     true,
		LogRemoteIP:      true,
		LogHost:          true,
		LogMethod:        true,
		LogURI:           true,
		LogUserAgent:     true,
		LogStatus:        true,
		LogError:         true,
		LogLatency:       true,
		LogContentLength: true,
		LogResponseSize:  true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			line := requestLogLine{
				Time:         time.Now().Format(time.RFC3339Nano),
				ID:           v.RequestID,
				RemoteIP:     v.RemoteIP,
				Host:         v.Host,
				Method:       v.Method,
				URI:          v.URI,
				UserAgent:    v.UserAgent,
				Status:       v.Status,
				Latency:      int64(v.Latency),
				LatencyHuman: v.Latency.String(),
				BytesOut:     v.ResponseSize,
			}
			if v.Error != nil {
				line.Error = v.Error.Error()
			}
			// Logger printed a missing Content-Length as 0.
			if v.ContentLength != "" {
				line.BytesIn, _ = strconv.ParseInt(v.ContentLength, 10, 64)
			}
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			// Logger left &, < and > in a URI or user agent unescaped.
			enc.SetEscapeHTML(false)
			if err := enc.Encode(line); err != nil {
				return err
			}
			_, err := c.Logger().Output().Write(buf.Bytes())
			return err
		},
	})
}
