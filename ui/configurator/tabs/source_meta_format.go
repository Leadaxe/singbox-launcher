package tabs

import (
	"fmt"
	"strings"
	"time"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
)

// sourceDiag — то, что строка списка знает про подписку: заголовки
// провайдера (SubMeta) и диагностика последней попытки (SubUpdateStatus).
//
// SPEC 118 W5: прежняя «мостовая Meta» держала обе половины в одной
// структуре; в каноне у них разные дома, и форматтеры берут снимок обеих.
// Указатели — потому что любая половина может отсутствовать (подписку ещё
// не обновляли; провайдер не прислал заголовков).
type sourceDiag struct {
	meta   *corestate.SubMeta
	status *corestate.SubUpdateStatus
}

// diagOf — снимок диагностики источника.
func diagOf(src *corestate.Source) *sourceDiag {
	if src == nil {
		return nil
	}
	if src.Meta == nil && src.UpdateStatus == nil {
		return nil
	}
	return &sourceDiag{meta: src.Meta, status: src.UpdateStatus}
}

func (d *sourceDiag) lastStatus() string {
	if d == nil || d.status == nil {
		return ""
	}
	return d.status.LastStatus
}

func (d *sourceDiag) errorCount() int {
	if d == nil || d.status == nil {
		return 0
	}
	return d.status.ErrorCount
}

func (d *sourceDiag) lastErrorMsg() string {
	if d == nil || d.status == nil {
		return ""
	}
	return d.status.LastErrorMsg
}

func (d *sourceDiag) lastAttemptAt() string {
	if d == nil || d.status == nil {
		return ""
	}
	return d.status.LastAttemptAt
}

func (d *sourceDiag) nodesCount() int {
	if d == nil || d.status == nil {
		return 0
	}
	return d.status.NodesCountFetched
}

func (d *sourceDiag) truncated() bool {
	return d != nil && d.status != nil && d.status.Truncated
}

func (d *sourceDiag) userInfo() *corestate.UserInfo {
	if d == nil || d.meta == nil {
		return nil
	}
	return d.meta.UserInfo
}

func (d *sourceDiag) profileTitle() string {
	if d == nil || d.meta == nil {
		return ""
	}
	return d.meta.ProfileTitle
}

func (d *sourceDiag) profileWebPageURL() string {
	if d == nil || d.meta == nil {
		return ""
	}
	return d.meta.ProfileWebPageURL
}

func (d *sourceDiag) supportURL() string {
	if d == nil || d.meta == nil {
		return ""
	}
	return d.meta.SupportURL
}

func (d *sourceDiag) providerAnnounce() *corestate.ProviderAnnounce {
	if d == nil || d.meta == nil {
		return nil
	}
	return d.meta.ProviderAnnounce
}

// formatStatusBadge возвращает текст статуса fetch для подписки.
//   - meta == nil или Empty → "● never"
//   - last_status == "ok" → "● ok"
//   - last_status == "err" → "● err"
func formatStatusBadge(meta *sourceDiag) string {
	if meta.lastStatus() == "" {
		return locale.T("● never")
	}
	switch meta.lastStatus() {
	case "ok":
		return locale.T("● ok")
	case "err":
		return locale.T("● err")
	}
	return locale.T("● never")
}

// formatLastFetched — relative time с момента LastFetchedAt.
//   - "" → "never fetched"
//   - < 1 минуты → "just fetched"
//   - иначе → "fetched 5m ago" / "fetched 2h ago" / "fetched 3d ago"
func formatLastFetched(meta *sourceDiag) string {
	if meta.lastAttemptAt() == "" {
		return locale.T("never fetched")
	}
	t, err := time.Parse(time.RFC3339, meta.lastAttemptAt())
	if err != nil {
		return locale.T("never fetched")
	}
	d := time.Since(t)
	if d < time.Minute {
		return locale.T("just fetched")
	}
	return locale.Tf("fetched %s ago", humanizeDuration(d))
}

// formatQuota — "1.2 GB / 50 GB used" если total > 0, иначе "".
func formatQuota(meta *sourceDiag) string {
	ui := meta.userInfo()
	if ui == nil {
		return ""
	}
	if ui.TotalBytes <= 0 {
		return ""
	}
	used := ui.UploadBytes + ui.DownloadBytes
	return locale.Tf("%s / %s used",
		humanizeBytes(used),
		humanizeBytes(ui.TotalBytes))
}

// quotaPercentage — used/total в [0..1]; 0 если нет квоты.
func quotaPercentage(meta *sourceDiag) float64 {
	ui := meta.userInfo()
	if ui == nil || ui.TotalBytes <= 0 {
		return 0
	}
	used := float64(ui.UploadBytes + ui.DownloadBytes)
	total := float64(ui.TotalBytes)
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
func formatExpire(meta *sourceDiag) string {
	ui := meta.userInfo()
	if ui == nil || ui.ExpireUnix <= 0 {
		return ""
	}
	expireAt := time.Unix(ui.ExpireUnix, 0)
	d := time.Until(expireAt)
	if d < 0 {
		return locale.T("expired")
	}
	return locale.Tf("expires in %s", humanizeDuration(d))
}

// formatNodesCount — "150 nodes" или "20000 nodes (truncated, max 3000)".
func formatNodesCount(meta *sourceDiag, effectiveMax int) string {
	n := meta.nodesCount()
	if n == 0 {
		return ""
	}
	if meta.truncated() && effectiveMax > 0 {
		return locale.Tf("%d nodes (truncated, max %d)", n, effectiveMax)
	}
	return locale.Tf("%d nodes", n)
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
func metaTooltip(meta *sourceDiag) string {
	if meta == nil {
		return ""
	}
	lines := []string{}
	if t := meta.profileTitle(); t != "" {
		lines = append(lines, "Title: "+t)
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
	if n := meta.nodesCount(); n > 0 {
		lines = append(lines, fmt.Sprintf("Nodes: %d", n))
		if meta.truncated() {
			lines = append(lines, "(truncated)")
		}
	}
	if u := meta.supportURL(); u != "" {
		lines = append(lines, "Support: "+u)
	}
	if meta.lastStatus() == "err" && meta.lastErrorMsg() != "" {
		lines = append(lines, "⚠ Last error: "+meta.lastErrorMsg())
		if c := meta.errorCount(); c > 0 {
			lines = append(lines, fmt.Sprintf("Error count: %d", c))
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
func formatSourceSubtitle(meta *sourceDiag, update *corestate.UpdateSpec, defaultReload string) string {
	if meta == nil {
		return ""
	}
	parts := []string{}

	if meta.lastStatus() == "err" {
		errMsg := "⚠"
		if c := meta.errorCount(); c > 0 {
			errMsg = fmt.Sprintf("⚠ %d", c)
		}
		parts = append(parts, errMsg)
	}

	if n := meta.nodesCount(); n > 0 {
		parts = append(parts, fmt.Sprintf("⁙ %d", n))
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

	if fetched := formatLastFetched(meta); fetched != "" && meta.lastAttemptAt() != "" {
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

// fetchWarningTexts — per-record деградации последнего fetch'а строками
// (SPEC 118 Т3: они персистятся в updateStatus, и UI читает их ИЗ СОСТОЯНИЯ,
// ничего не перепарсивая).
//
// Это и есть ответ на «почему узлов столько»: пропущенные skip'ом записи,
// битые элементы тела, потерянные группы-члены. До SPEC 118 их знал только
// разбор, который вкладка Preview делала повторно своим кодом; теперь тела
// разбираются один раз, и второго источника этих строк не существует.
//
// Формат — тот же, что у отчёта сборки (build_report_feed): адресация тегом
// узла, счётчик у агрегируемых видов. Расходиться им нельзя: пользователь
// читает про одно и то же событие в двух местах.
func fetchWarningTexts(st *corestate.SubUpdateStatus) []string {
	if st == nil || len(st.Warnings) == 0 {
		return nil
	}
	out := make([]string, 0, len(st.Warnings))
	for _, w := range st.Warnings {
		reason := w.Message
		if reason == "" {
			reason = w.Kind
		}
		if w.Tag != "" {
			reason = w.Tag + ": " + reason
		}
		if w.Count > 1 {
			reason = fmt.Sprintf("%s (×%d)", reason, w.Count)
		}
		out = append(out, reason)
	}
	return out
}
