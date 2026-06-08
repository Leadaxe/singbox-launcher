package dialogs

import "singbox-launcher/internal/srstag"

// SRSTagFromURL — content-addressed SRS tag (filename-without-.srs + hash8 of
// the URL), shared with the build pipeline via internal/srstag.TagFromURL —
// the single source of truth (the downloader, the build resolver and this UI
// must agree on the bin/rule-sets/<tag>.srs key). Kept as a package-local
// wrapper so existing dialogs/* and tabs/* callers don't import srstag directly.
func SRSTagFromURL(urlStr string) string { return srstag.TagFromURL(urlStr) }

func srsTagFromURL(urlStr string) string { return srstag.TagFromURL(urlStr) }
