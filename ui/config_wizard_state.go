package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
)

// safeFyneDo safely calls fyne.Do only if window is still valid.
func safeFyneDo(window fyne.Window, fn func()) {
	if window != nil {
		fyne.Do(fn)
	}
}

// debugLog logs a debug message using the debuglog subsystem.
func debugLog(format string, args ...interface{}) {
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, format, args...)
}

// infoLog logs an info message using the debuglog subsystem.
func infoLog(format string, args ...interface{}) {
	debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, format, args...)
}

// errorLog logs an error message using the debuglog subsystem.
func errorLog(format string, args ...interface{}) {
	debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, format, args...)
}

// WizardState хранит состояние мастера конфигурации.
type WizardState struct {
	Controller *core.AppController
	Window     fyne.Window

	// Tab 1: VLESS Sources
	VLESSURLEntry        *widget.Entry
	URLStatusLabel       *widget.Label
	ParserConfigEntry    *widget.Entry
	OutboundsPreview     *widget.Entry
	OutboundsPreviewText string // Храним текст для read-only режима
	CheckURLButton       *widget.Button
	CheckURLProgress     *widget.ProgressBar
	CheckURLPlaceholder  *canvas.Rectangle
	CheckURLContainer    fyne.CanvasObject
	checkURLInProgress   bool
	ParseButton          *widget.Button
	parserConfigUpdating bool

	// Parsed data
	ParserConfig       *config.ParserConfig
	GeneratedOutbounds []string
	// Statistics for preview (used when nodes > maxNodesForFullPreview)
	OutboundStats struct {
		NodesCount           int
		LocalSelectorsCount  int
		GlobalSelectorsCount int
	}

	// Template data for second tab
	TemplateData                *TemplateData
	TemplateSectionSelections   map[string]bool
	SelectableRuleStates        []*SelectableRuleState
	CustomRules                 []*SelectableRuleState // User-defined custom rules
	TemplatePreviewEntry        *widget.Entry
	TemplatePreviewText         string
	TemplatePreviewStatusLabel  *widget.Label
	ShowPreviewButton           *widget.Button
	FinalOutboundSelect         *widget.Select
	SelectedFinalOutbound       string
	previewNeedsParse           bool
	templatePreviewNeedsUpdate  bool // Флаг для пересчета превью при изменениях на вкладке Rules
	autoParseInProgress         bool
	previewGenerationInProgress bool

	// Flag to prevent callbacks during programmatic updates
	updatingOutboundOptions bool

	// Navigation buttons
	CloseButton      *widget.Button
	PrevButton       *widget.Button
	NextButton       *widget.Button
	SaveButton       *widget.Button
	SaveProgress     *widget.ProgressBar
	SavePlaceholder  *canvas.Rectangle
	saveInProgress   bool
	ButtonsContainer fyne.CanvasObject
	tabs             *container.AppTabs

	// Track open rule edit dialogs to prevent multiple dialogs for the same rule
	openRuleDialogs map[int]fyne.Window // ruleIndex -> dialog window
}

// SelectableRuleState описывает состояние переключаемого правила.
type SelectableRuleState struct {
	Rule             TemplateSelectableRule
	Enabled          bool
	SelectedOutbound string
	OutboundSelect   *widget.Select
}

const (
	defaultOutboundTag = "direct-out"
	rejectActionName   = "reject"
	rejectActionMethod = "drop"
)
