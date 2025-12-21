package utils

import "time"

// Timeouts for network operations
const (
	// HTTPRequestTimeout is the timeout for HTTP requests
	HTTPRequestTimeout = 30 * time.Second
	// SubscriptionFetchTimeout is the timeout for fetching subscriptions
	SubscriptionFetchTimeout = 60 * time.Second
	// URIParseTimeout is the timeout for parsing URIs
	URIParseTimeout = 10 * time.Second
	// MaxWaitTime is the maximum time to wait for operations
	MaxWaitTime = 60 * time.Second
)

// Size limits for data validation
const (
	// MaxSubscriptionSize is the maximum size of subscription response (10MB)
	MaxSubscriptionSize = 10 * 1024 * 1024
	// MaxJSONConfigSize is the maximum size of JSON configuration
	MaxJSONConfigSize = 50 * 1024 * 1024
	// MaxURILength is the maximum length of URI for parsing
	MaxURILength = 8192
	// MinURILength is the minimum length of URI for validation
	MinURILength = 10
)

// UI constants
const (
	// MaxNodesForFullPreview is the maximum number of nodes to show full preview
	// If nodes count exceeds this value, statistics comment will be shown instead
	MaxNodesForFullPreview = 20
	// WizardWindowWidth is the default width of wizard window
	WizardWindowWidth = 620
	// WizardWindowHeight is the default height of wizard window
	WizardWindowHeight = 660
	// PreviewTextThreshold is the maximum number of preview lines to show
	PreviewTextThreshold = 10
	// MaxPreviewLines is the maximum number of lines in preview
	MaxPreviewLines = 10
)

// UI spacing and sizing
const (
	// UIPaddingRight is the padding from right edge in Fyne units
	UIPaddingRight = 10
	// ParserHeight is the height for parser entry (~10 lines)
	ParserHeight = 200
)

// Progress update intervals
const (
	// ProgressUpdateInterval is the minimum interval between progress updates
	ProgressUpdateInterval = 200 * time.Millisecond
	// ShortDelay is a short delay for UI updates
	ShortDelay = 100 * time.Millisecond
	// MediumDelay is a medium delay for UI updates
	MediumDelay = 200 * time.Millisecond
)


