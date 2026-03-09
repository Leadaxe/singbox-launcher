// Package models содержит модели данных визарда конфигурации.
//
// Файл wizard_state_file.go определяет структуры данных для сериализации состояния визарда в JSON.
//
// WizardStateFile — основная структура для сохранения/загрузки state.json:
//   - Метаданные (version, id, comment, created_at, updated_at)
//   - ParserConfig — конфигурация парсера (в памяти как config.ParserConfig, в JSON — упрощенная структура без обертки)
//   - ConfigParams — параметры конфигурации (route.final и др.)
//   - SelectableRuleStates — упрощённые состояния правил из шаблона (только label, enabled, selected_outbound)
//   - CustomRules — пользовательские правила (полная структура)
//
// Selectable rules хранят только выбор пользователя — определение правила берётся из шаблона.
// Custom rules хранят полную структуру, т.к. они не привязаны к шаблону.
//
// Поддерживается миграция со старого формата state.json:
//   - selectable_rule_states содержали вложенный объект rule с полным определением правила
//   - parser_config содержал обертку ParserConfig (теперь упрощенная структура без обертки)
//
// Используется в:
//   - business/state_store.go — для сохранения/загрузки состояний
//   - presentation/presenter_state.go — для создания состояния из модели
package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"singbox-launcher/core/config"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

const (
	// WizardStateVersion — версия формата файла состояния.
	WizardStateVersion = 2

	// MaxStateIDLength — максимальная длина ID состояния.
	MaxStateIDLength = 50

	// StateFileName — имя файла текущего состояния.
	StateFileName = "state.json"
)

var (
	// stateIDRegex — допустимые символы для ID состояния.
	stateIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// WizardStateFile представляет сериализуемое состояние визарда.
type WizardStateFile struct {
	Version              int                            `json:"version"`
	ID                   string                         `json:"id,omitempty"`
	Comment              string                         `json:"comment,omitempty"`
	CreatedAt            time.Time                      `json:"created_at"`
	UpdatedAt            time.Time                      `json:"updated_at"`
	ParserConfig         config.ParserConfig            `json:"-"` // Используется только в памяти, не сериализуется напрямую
	ConfigParams         []ConfigParam                  `json:"config_params"`
	SelectableRuleStates []PersistedSelectableRuleState `json:"selectable_rule_states"`
	CustomRules          []PersistedCustomRule          `json:"custom_rules"`
}

// ConfigParam представляет параметр конфигурации.
type ConfigParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PersistedSelectableRuleState — упрощённое состояние selectable rule.
// Правило определяется шаблоном, здесь хранится только выбор пользователя.
type PersistedSelectableRuleState struct {
	Label            string `json:"label"`
	Enabled          bool   `json:"enabled"`
	SelectedOutbound string `json:"selected_outbound"`
}

// PersistedCustomRule — полное определение пользовательского правила.
type PersistedCustomRule struct {
	Label            string                 `json:"label"`
	Type             string                 `json:"type,omitempty"`
	Enabled          bool                   `json:"enabled"`
	SelectedOutbound string                 `json:"selected_outbound"`
	Description      string                 `json:"description,omitempty"`
	Rule             map[string]interface{} `json:"rule,omitempty"`
	DefaultOutbound  string                 `json:"default_outbound,omitempty"`
	HasOutbound      bool                   `json:"has_outbound"`
}

// WizardStateMetadata — метаданные состояния для списка (без полного содержимого).
type WizardStateMetadata struct {
	ID        string    `json:"id"`
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	IsCurrent bool      `json:"is_current"`
}

// ValidateStateID проверяет валидность ID состояния.
func ValidateStateID(id string) error {
	if id == "" {
		return fmt.Errorf("state ID cannot be empty")
	}
	if len(id) > MaxStateIDLength {
		return fmt.Errorf("state ID exceeds maximum length of %d characters", MaxStateIDLength)
	}
	if !stateIDRegex.MatchString(id) {
		return fmt.Errorf("state ID can only contain letters (a-z, A-Z), numbers (0-9), hyphen (-), and underscore (_)")
	}
	return nil
}

// ToPersistedSelectableRuleState конвертирует RuleState в упрощённый формат для сохранения.
func ToPersistedSelectableRuleState(ruleState *RuleState) PersistedSelectableRuleState {
	return PersistedSelectableRuleState{
		Label:            ruleState.Rule.Label,
		Enabled:          ruleState.Enabled,
		SelectedOutbound: ruleState.SelectedOutbound,
	}
}

// ToPersistedCustomRule конвертирует RuleState (custom rule) в формат для сохранения.
func ToPersistedCustomRule(ruleState *RuleState) PersistedCustomRule {
	ruleType := DetermineRuleType(ruleState.Rule.Rule)
	return PersistedCustomRule{
		Label:            ruleState.Rule.Label,
		Type:             ruleType,
		Enabled:          ruleState.Enabled,
		SelectedOutbound: ruleState.SelectedOutbound,
		Description:      ruleState.Rule.Description,
		Rule:             ruleState.Rule.Rule,
		DefaultOutbound:  ruleState.Rule.DefaultOutbound,
		HasOutbound:      ruleState.Rule.HasOutbound,
	}
}

// ToRuleState конвертирует PersistedCustomRule в RuleState.
func (pcr *PersistedCustomRule) ToRuleState() *RuleState {
	return &RuleState{
		Rule: wizardtemplate.TemplateSelectableRule{
			Label:           pcr.Label,
			Description:     pcr.Description,
			Rule:            pcr.Rule,
			DefaultOutbound: pcr.DefaultOutbound,
			HasOutbound:     pcr.HasOutbound,
		},
		Enabled:          pcr.Enabled,
		SelectedOutbound: pcr.SelectedOutbound,
	}
}

// NewWizardStateFile создает новый WizardStateFile из компонентов.
// Инкапсулирует логику работы с ParserConfig, скрывая детали реализации от UI.
//
// Параметры:
//   - parserConfigRaw: упрощенная структура parser_config (без обертки ParserConfig) в виде JSON
//   - configParams: параметры конфигурации
//   - selectableRuleStates: состояния selectable rules
//   - customRules: пользовательские правила
//
// Возвращает готовый WizardStateFile с правильно упакованным ParserConfig.
func NewWizardStateFile(
	parserConfigRaw json.RawMessage,
	configParams []ConfigParam,
	selectableRuleStates []PersistedSelectableRuleState,
	customRules []PersistedCustomRule,
) (*WizardStateFile, error) {
	// Парсим parser_config в map для обработки
	var parserConfigData map[string]interface{}
	if len(parserConfigRaw) > 0 {
		if err := json.Unmarshal(parserConfigRaw, &parserConfigData); err != nil {
			return nil, fmt.Errorf("failed to parse parser_config: %w", err)
		}
	}

	// Оборачиваем в структуру ParserConfig для совместимости с внутренним форматом
	var parserConfig config.ParserConfig
	if len(parserConfigData) > 0 {
		wrappedConfig := map[string]interface{}{
			"ParserConfig": parserConfigData,
		}
		wrappedJSON, err := json.Marshal(wrappedConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap parser_config: %w", err)
		}

		if err := json.Unmarshal(wrappedJSON, &parserConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal parser_config: %w", err)
		}
	}

	// Инициализируем пустые slice, если они nil
	if configParams == nil {
		configParams = []ConfigParam{}
	}
	if selectableRuleStates == nil {
		selectableRuleStates = []PersistedSelectableRuleState{}
	}
	if customRules == nil {
		customRules = []PersistedCustomRule{}
	}

	// Создаем WizardStateFile
	now := time.Now().UTC()
	return &WizardStateFile{
		Version:              WizardStateVersion,
		ParserConfig:         parserConfig,
		ConfigParams:         configParams,
		SelectableRuleStates: selectableRuleStates,
		CustomRules:          customRules,
		CreatedAt:            now,
		UpdatedAt:            now,
	}, nil
}

// DetermineRuleType определяет тип правила на основе содержимого.
func DetermineRuleType(rule map[string]interface{}) string {
	if rule == nil {
		return "Custom JSON"
	}
	if _, ok := rule["ip_cidr"]; ok {
		return "IP Addresses (CIDR)"
	}
	if _, ok := rule["domain_regex"]; ok {
		return "Domains/URLs"
	}
	if _, ok := rule["domain"]; ok {
		return "Domains/URLs"
	}
	if _, ok := rule["domain_suffix"]; ok {
		return "Domains/URLs"
	}
	if _, ok := rule["domain_keyword"]; ok {
		return "Domains/URLs"
	}
	if _, ok := rule["process_name"]; ok {
		return "Processes"
	}
	if _, ok := rule["process_path_regex"]; ok {
		return "Processes"
	}
	return "System"
}

// MigrateSelectableRuleStates мигрирует selectable_rule_states из старого формата.
// Старый формат: [{rule: {label: "X", ...}, enabled: true, selected_outbound: "Y"}]
// Новый формат: [{label: "X", enabled: true, selected_outbound: "Y"}]
func MigrateSelectableRuleStates(raw json.RawMessage) []PersistedSelectableRuleState {
	// Пробуем новый формат
	var newFormat []PersistedSelectableRuleState
	if err := json.Unmarshal(raw, &newFormat); err == nil {
		// Проверяем, что labels заполнены (в новом формате label на верхнем уровне)
		if len(newFormat) > 0 && newFormat[0].Label != "" {
			return newFormat
		}
	}

	// Пробуем старый формат с вложенным rule
	var oldFormat []struct {
		Enabled          bool   `json:"enabled"`
		SelectedOutbound string `json:"selected_outbound"`
		Rule             struct {
			Label string `json:"label"`
		} `json:"rule"`
	}
	if err := json.Unmarshal(raw, &oldFormat); err == nil {
		result := make([]PersistedSelectableRuleState, 0, len(oldFormat))
		for _, old := range oldFormat {
			label := old.Rule.Label
			if label == "" {
				continue
			}
			result = append(result, PersistedSelectableRuleState{
				Label:            label,
				Enabled:          old.Enabled,
				SelectedOutbound: old.SelectedOutbound,
			})
		}
		return result
	}

	return nil
}

// MigrateCustomRules мигрирует custom_rules из старого формата.
// Старый формат: [{type: "X", rule: {label: "Y", raw: {...}, ...}, enabled: true}]
// Новый формат: [{label: "Y", type: "X", rule: {...}, enabled: true}]
func MigrateCustomRules(raw json.RawMessage) []PersistedCustomRule {
	// Пробуем новый формат
	var newFormat []PersistedCustomRule
	if err := json.Unmarshal(raw, &newFormat); err == nil {
		if len(newFormat) > 0 && newFormat[0].Label != "" {
			return newFormat
		}
	}

	// Пробуем старый формат
	var oldFormat []struct {
		Type             string `json:"type"`
		Enabled          bool   `json:"enabled"`
		SelectedOutbound string `json:"selected_outbound"`
		Rule             struct {
			Label           string                 `json:"label"`
			Description     string                 `json:"description"`
			Raw             map[string]interface{} `json:"raw"`
			DefaultOutbound string                 `json:"default_outbound"`
			HasOutbound     bool                   `json:"has_outbound"`
		} `json:"rule"`
	}
	if err := json.Unmarshal(raw, &oldFormat); err == nil {
		result := make([]PersistedCustomRule, 0, len(oldFormat))
		for _, old := range oldFormat {
			result = append(result, PersistedCustomRule{
				Label:            old.Rule.Label,
				Type:             old.Type,
				Enabled:          old.Enabled,
				SelectedOutbound: old.SelectedOutbound,
				Description:      old.Rule.Description,
				Rule:             old.Rule.Raw,
				DefaultOutbound:  old.Rule.DefaultOutbound,
				HasOutbound:      old.Rule.HasOutbound,
			})
		}
		return result
	}

	return nil
}

// MarshalJSON кастомная сериализация для правильного формата времени и упрощенной структуры parser_config.
func (wsf *WizardStateFile) MarshalJSON() ([]byte, error) {
	// Извлекаем содержимое из ParserConfig.ParserConfig для упрощенной структуры
	var parserConfigRaw json.RawMessage
	if wsf.ParserConfig.ParserConfig.Proxies != nil {
		// Сериализуем только содержимое ParserConfig (без обертки)
		raw, err := json.Marshal(wsf.ParserConfig.ParserConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal parser_config: %w", err)
		}
		parserConfigRaw = raw
	}

	type Alias WizardStateFile
	return json.Marshal(&struct {
		*Alias
		CreatedAt    string          `json:"created_at"`
		UpdatedAt    string          `json:"updated_at"`
		ParserConfig json.RawMessage `json:"parser_config"`
	}{
		Alias:        (*Alias)(wsf),
		CreatedAt:    wsf.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    wsf.UpdatedAt.Format(time.RFC3339),
		ParserConfig: parserConfigRaw,
	})
}

// UnmarshalJSON кастомная десериализация с поддержкой миграции и упрощенной структуры parser_config.
func (wsf *WizardStateFile) UnmarshalJSON(data []byte) error {
	// Десериализуем базовые поля
	type BasicFields struct {
		Version      int             `json:"version"`
		ID           string          `json:"id,omitempty"`
		Comment      string          `json:"comment,omitempty"`
		CreatedAt    string          `json:"created_at"`
		UpdatedAt    string          `json:"updated_at"`
		ParserConfig json.RawMessage `json:"parser_config"` // Упрощенная структура или старая с оберткой
		ConfigParams []ConfigParam   `json:"config_params"`
		// raw messages для миграции
		SelectableRuleStates json.RawMessage `json:"selectable_rule_states"`
		CustomRules          json.RawMessage `json:"custom_rules"`
	}

	var basic BasicFields
	if err := json.Unmarshal(data, &basic); err != nil {
		return err
	}

	wsf.Version = basic.Version
	wsf.ID = basic.ID
	wsf.Comment = basic.Comment
	wsf.ConfigParams = basic.ConfigParams

	// Парсим время
	if basic.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, basic.CreatedAt); err == nil {
			wsf.CreatedAt = t
		}
	}
	if basic.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, basic.UpdatedAt); err == nil {
			wsf.UpdatedAt = t
		}
	}

	// Парсим parser_config: поддерживаем как упрощенную структуру, так и старую с оберткой ParserConfig
	if len(basic.ParserConfig) > 0 {
		// Пробуем упрощенную структуру (без обертки ParserConfig)
		var simplified struct {
			Version   int                     `json:"version"`
			Proxies   []config.ProxySource    `json:"proxies"`
			Outbounds []config.OutboundConfig `json:"outbounds"`
			Parser    struct {
				Reload      string `json:"reload,omitempty"`
				LastUpdated string `json:"last_updated,omitempty"`
			} `json:"parser,omitempty"`
		}
		if err := json.Unmarshal(basic.ParserConfig, &simplified); err == nil && simplified.Proxies != nil {
			// Упрощенная структура - оборачиваем в ParserConfig
			wsf.ParserConfig.ParserConfig.Version = simplified.Version
			wsf.ParserConfig.ParserConfig.Proxies = simplified.Proxies
			wsf.ParserConfig.ParserConfig.Outbounds = simplified.Outbounds
			wsf.ParserConfig.ParserConfig.Parser = simplified.Parser
		} else {
			// Старая структура с оберткой ParserConfig - парсим как есть
			var oldFormat config.ParserConfig
			if err := json.Unmarshal(basic.ParserConfig, &oldFormat); err == nil {
				wsf.ParserConfig = oldFormat
			} else {
				return fmt.Errorf("failed to parse parser_config: %w", err)
			}
		}
	}

	// Мигрируем selectable_rule_states
	if len(basic.SelectableRuleStates) > 0 {
		wsf.SelectableRuleStates = MigrateSelectableRuleStates(basic.SelectableRuleStates)
	}

	// Мигрируем custom_rules
	if len(basic.CustomRules) > 0 {
		wsf.CustomRules = MigrateCustomRules(basic.CustomRules)
	}

	return nil
}

// MarshalJSON кастомная сериализация для WizardStateMetadata.
func (wsm *WizardStateMetadata) MarshalJSON() ([]byte, error) {
	type Alias WizardStateMetadata
	return json.Marshal(&struct {
		*Alias
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}{
		Alias:     (*Alias)(wsm),
		CreatedAt: wsm.CreatedAt.Format(time.RFC3339),
		UpdatedAt: wsm.UpdatedAt.Format(time.RFC3339),
	})
}

// UnmarshalJSON кастомная десериализация для WizardStateMetadata.
func (wsm *WizardStateMetadata) UnmarshalJSON(data []byte) error {
	type Alias WizardStateMetadata
	aux := &struct {
		*Alias
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}{
		Alias: (*Alias)(wsm),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, aux.CreatedAt); err == nil {
			wsm.CreatedAt = t
		}
	}
	if aux.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, aux.UpdatedAt); err == nil {
			wsm.UpdatedAt = t
		}
	}
	return nil
}
