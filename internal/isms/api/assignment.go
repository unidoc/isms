package api

import (
	"strings"

	"github.com/labstack/echo/v4"
)

// canActOnAssignment reports whether the requesting user may act on an item
// assigned to assignee: a manager or admin always, a contributor only on an
// item assigned to them (#23 for task status, #203 for corrective actions and
// task notes). Emails compare case-insensitively: stored assignees are
// lowercase, but the Cloudflare Access path sets user_email unnormalised.
func canActOnAssignment(c echo.Context, assignee string) bool {
	switch role, _ := c.Get("user_role").(string); role {
	case "admin", "manager":
		return true
	case "contributor":
		return assignee != "" && strings.EqualFold(assignee, getUserEmail(c))
	}
	return false
}

// isManagerRole reports whether the requesting user is a manager or admin.
func isManagerRole(c echo.Context) bool {
	role, _ := c.Get("user_role").(string)
	return role == "admin" || role == "manager"
}
