package tabs

import (
	"fmt"
	"strings"
	"time"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
)

// formatStatusBadge возвращает текст статуса fetch для подписки.
//   - meta == nil или Empty → "● never"
//   - last_status == "ok" → "● ok"
//   - last_status == "err" → "● err"
func formatStatusBadge(meta *corestate.SubscriptionMeta) string {
	if meta == nil || meta.LastStatus == "" {
		return locale.T("wizard.source.status_never")
	}
	switch meta.LastStatus {
	case "ok":
		return locale.T("wizard.source.status_ok")
	case "err":
		return locale.T("wizard.source.status_err")
	}
	return locale.T("wizard.source.status_never")
}

// formatLastFetched — relative time с момента LastFetchedAt.
//   - "" → "never fetched"
//   - < 1 минуты → "just fetched"
//   - иначе → "fetched 5m ago" / "fetched 2h ago" / "fetched 3d ago"
func formatLastFetched(meta *corestate.SubscriptionMeta) string {
	if meta == nil || meta.LastFetchedAt == "" {
		return locale.T("wizard.source.meta_never_fetched")
	}
	t, err := time.Parse(time.RFC3339, meta.LastFetchedAt)
	if err != nil {
		return locale.T("wizard.source.meta_never_fetched")
	}
	d := time.Since(t)
	if d < time.Minute {
		return locale.T("wizard.source.meta_just_fetched")
	}
	return locale.Tf("wizard.source.meta_last_fetched", humanizeDuration(d))
}

// formatQuota — "1.2 GB / 50 GB used" если total > 0, иначе "".
func formatQuota(meta *corestate.SubscriptionMeta) string {
	if meta == nil || meta.UserInfo == nil {
		return ""
	}
	ui := meta.UserInfo
	if ui.TotalBytes <= 0 {
		return ""
	}
	used := ui.UploadBytes + ui.DownloadBytes
	return locale.Tf("wizard.source.meta_quota",
		humanizeBytes(used),
		humanizeBytes(ui.TotalBytes))
}

// quotaPercentage — used/total в [0..1]; 0 если нет квоты.
func quotaPercentage(meta *corestate.SubscriptionMeta) float64 {
	if meta == nil || meta.UserInfo == nil || meta.UserInfo.TotalBytes <= 0 {
		return 0
	}
	used := float64(meta.UserInfo.UploadBytes + meta.UserInfo.DownloadBytes)
	total := float64(meta.UserInfo.TotalBytes)
	if total == 0 {
		return 0
	}
	pct := used / total
	if pct < 0 {
		return 0
	}
	if pct > 1 {
		return 1
	}
	return pct
}

// formatExpire — "expires in 12 days" / "expired" / "" если нет данных.
func formatExpire(meta *corestate.SubscriptionMeta) string {
	if meta == nil || meta.UserInfo == nil || meta.UserInfo.ExpireUnix <= 0 {
		return ""
	}
	expireAt := time.Unix(meta.UserInfo.ExpireUnix, 0)
	d := time.Until(expireAt)
	if d < 0 {
		return locale.T("wizard.source.meta_expired")
	}
	return locale.Tf("wizard.source.meta_expires_in", humanizeDuration(d))
}

// formatNodesCount — "150 nodes" или "20000 nodes (truncated, max 3000)".
func formatNodesCount(meta *corestate.SubscriptionMeta, effectiveMax int) string {
	if meta == nil {
		return ""
	}
	if meta.NodesCountFetched == 0 {
		return ""
	}
	if meta.Truncated && effectiveMax > 0 {
		return locale.Tf("wizard.source.meta_truncated", meta.NodesCountFetched, effectiveMax)
	}
	return locale.Tf("wizard.source.meta_nodes_count", meta.NodesCountFetched)
}

// humanizeBytes — "1.2 GB" / "150 MB" / "5 KB". Используем 1024-base.
func humanizeBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case n >= TB:
		return fmt.Sprintf("%.2f TB", float64(n)/TB)
	case n >= GB:
		return fmt.Sprintf("%.2f GB", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/MB)
	case n >= KB:
		return fmt.Sprintf("%.0f KB", float64(n)/KB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// humanizeDuration — "5m" / "2h" / "3 days" / "12 days".
func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 365*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d days", days)
	default:
		years := d.Hours() / 24 / 365
		return fmt.Sprintf("%.1f years", years)
	}
}

// metaTooltip собирает многострочный tooltip для meta-info. "Status: ok"
// не дублируется (subtitle и так показывает fetched-time + ⚠ при ошибках).
func metaTooltip(meta *corestate.SubscriptionMeta) string {
	if meta == nil {
		return ""
	}
	lines := []string{}
	if meta.ProfileTitle != "" {
		lines = append(lines, "Title: "+meta.ProfileTitle)
	}
	if fetched := formatLastFetched(meta); fetched != "" {
		lines = append(lines, "Fetched: "+fetched)
	}
	if quota := formatQuota(meta); quota != "" {
		lines = append(lines, "Quota: "+quota)
	}
	if expires := formatExpire(meta); expires != "" {
		lines = append(lines, "Expires: "+expires)
	}
	if meta.NodesCountFetched > 0 {
		lines = append(lines, fmt.Sprintf("Nodes: %d", meta.NodesCountFetched))
		if meta.Truncated {
			lines = append(lines, "(truncated)")
		}
	}
	if meta.SupportURL != "" {
		lines = append(lines, "Support: "+meta.SupportURL)
	}
	if meta.LastStatus == "err" && meta.LastErrorMsg != "" {
		lines = append(lines, "⚠ Last error: "+meta.LastErrorMsg)
		if meta.ErrorCount > 0 {
			lines = append(lines, fmt.Sprintf("Error count: %d", meta.ErrorCount))
		}
	}
	return strings.Join(lines, "\n")
}

// formatSourceSubtitle — единичная строка с meta-инфой для отображения
// под title-строкой source-row'а: ⚠ X errors  •  📊 N nodes  •  ⏱ Xh  •  🕒 5m ago.
//
// Возвращает "" если нет полезной информации (для server-type / новой
// подписки без meta — subtitle строка не рендерится). Error-case ставится
// первым с ⚠ и сообщением — чтобы пользователь сразу видел проблему.
func formatSourceSubtitle(meta *corestate.SubscriptionMeta, update *corestate.UpdateSpec, defaultReload string) string {
	if meta == nil {
		return ""
	}
	parts := []string{}

	if meta.LastStatus == "err" {
		errMsg := "⚠"
		if meta.ErrorCount > 0 {
			errMsg = fmt.Sprintf("⚠ %d", meta.ErrorCount)
		}
		parts = append(parts, errMsg)
	}

	if meta.NodesCountFetched > 0 {
		parts = append(parts, fmt.Sprintf("⁙ %d", meta.NodesCountFetched))
	}

	interval := ""
	if update != nil && update.IntervalHours > 0 {
		interval = fmt.Sprintf("%dh", update.IntervalHours)
	} else if defaultReload != "" {
		interval = defaultReload
	}
	if interval != "" {
		parts = append(parts, "↻ "+interval)
	}

	if fetched := formatLastFetched(meta); fetched != "" && meta.LastFetchedAt != "" {
		parts = append(parts, "🕒 "+fetched)
	}

	if quota := formatQuota(meta); quota != "" {
		parts = append(parts, "💾 "+quota)
	}
	if expires := formatExpire(meta); expires != "" {
		parts = append(parts, "⏳ "+expires)
	}

	return strings.Join(parts, "  •  ")
}

