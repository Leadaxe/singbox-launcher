# Рекомендации по оптимизации кода после рефакторинга UI визарда

## Контекст

После рефакторинга `ui/config_wizard.go` (2284 строки) в модульную структуру `ui/wizard/` объем кода увеличился с ~2,750 до ~5,381 строк (+2,631 строка). Анализ показал, что ~60% увеличения - это улучшения качества (тесты, валидация), но ~40% можно оптимизировать.

## Статистика раздувания

- **Всего строк в `ui/wizard/`**: ~5,381
- **Тесты**: ~1,367 строк (24 функции) - это улучшение качества
- **Логирование**: ~500 строк (117 вызовов DebugLog)
- **Измерение времени**: ~300 строк (92 упоминания startTime/time.Since)
- **Валидация**: ~192 строки (новый модуль - требование)
- **Thread-safety обертки**: ~200 строк (43 вызова SafeFyneDo)
- **Godoc комментарии**: ~300 строк (138 комментариев)
- **Overhead модулей**: ~300 строк (package declarations, imports)

## Рекомендации по оптимизации

### 1. Условное логирование (экономия ~300 строк)

**Проблема:** 117 вызовов `DebugLog` всегда выполняются, даже в production.

**Решение:** Добавить флаг уровня логирования в `wizard/state/helpers.go`:

```go
package state

import (
	"fyne.io/fyne/v2"
	"singbox-launcher/internal/debuglog"
)

// LogLevel controls the verbosity of logging
type LogLevel int

const (
	LogLevelOff LogLevel = iota
	LogLevelError
	LogLevelInfo
	LogLevelDebug
)

var currentLogLevel = LogLevelInfo // Default: only errors and info

// SetLogLevel sets the current logging level
func SetLogLevel(level LogLevel) {
	currentLogLevel = level
}

// SafeFyneDo safely calls fyne.Do only if window is still valid.
func SafeFyneDo(window fyne.Window, fn func()) {
	if window != nil {
		fyne.Do(fn)
	}
}

// DebugLog logs a debug message only if log level is Debug or higher.
func DebugLog(format string, args ...interface{}) {
	if currentLogLevel >= LogLevelDebug {
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, format, args...)
	}
}

// InfoLog logs an info message only if log level is Info or higher.
func InfoLog(format string, args ...interface{}) {
	if currentLogLevel >= LogLevelInfo {
		debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, format, args...)
	}
}

// ErrorLog always logs error messages.
func ErrorLog(format string, args ...interface{}) {
	debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, format, args...)
}
```

**Использование:**
```go
// В main.go или при инициализации
wizardstate.SetLogLevel(wizardstate.LogLevelError) // Production: только ошибки
// wizardstate.SetLogLevel(wizardstate.LogLevelDebug) // Development: все логи
```

**Экономия:** ~300 строк логирования можно отключить в production.

**Приоритет:** ВЫСОКИЙ

---

### 2. Утилита для измерения времени (экономия ~200 строк)

**Проблема:** Повторяющийся паттерн `startTime := time.Now()` + `time.Since(startTime)` в каждой функции.

**Решение:** Создать `wizard/utils/timing.go`:

```go
package utils

import (
	"time"
	wizardstate "singbox-launcher/ui/wizard/state"
)

// TimingContext tracks timing for a function execution
type TimingContext struct {
	startTime time.Time
	funcName  string
}

// StartTiming creates a new timing context and logs start
func StartTiming(funcName string) *TimingContext {
	startTime := time.Now()
	wizardstate.DebugLog("%s: START at %s", funcName, startTime.Format("15:04:05.000"))
	return &TimingContext{
		startTime: startTime,
		funcName:  funcName,
	}
}

// LogTiming logs elapsed time for a specific operation
func (tc *TimingContext) LogTiming(operation string, duration time.Duration) {
	wizardstate.DebugLog("%s: %s took %v", tc.funcName, operation, duration)
}

// End logs total duration and returns it
func (tc *TimingContext) End() time.Duration {
	duration := time.Since(tc.startTime)
	wizardstate.DebugLog("%s: END (total duration: %v)", tc.funcName, duration)
	return duration
}

// EndWithDefer returns a defer function for automatic logging
func (tc *TimingContext) EndWithDefer() func() {
	return func() {
		tc.End()
	}
}
```

**Использование:**
```go
// Было (10 строк):
startTime := time.Now()
wizardstate.DebugLog("checkURL: START at %s", startTime.Format("15:04:05.000"))
// ... код ...
fetchStartTime := time.Now()
content, err := subscription.FetchSubscription(line)
fetchDuration := time.Since(fetchStartTime)
wizardstate.DebugLog("checkURL: Fetched subscription (took %v)", fetchDuration)
// ... код ...
wizardstate.DebugLog("checkURL: END (total duration: %v)", time.Since(startTime))

// Стало (5 строк):
timing := wizardutils.StartTiming("checkURL")
defer timing.EndWithDefer()
// ... код ...
fetchTiming := wizardutils.StartTiming("fetchSubscription")
content, err := subscription.FetchSubscription(line)
timing.LogTiming("fetchSubscription", fetchTiming.End())
```

**Экономия:** ~200 строк повторяющегося кода.

**Приоритет:** СРЕДНИЙ

---

### 3. Упрощение SafeFyneDo (экономия ~100 строк)

**Проблема:** 43 вызова `SafeFyneDo` с анонимными функциями создают избыточный boilerplate.

**Решение:** Добавить helper-методы в `WizardState`:

```go
// В wizard/state/state.go добавить методы:

// UpdateUI safely updates UI elements
func (state *WizardState) UpdateUI(fn func()) {
	SafeFyneDo(state.Window, fn)
}

// SetURLStatus safely sets URL status label
func (state *WizardState) SetURLStatus(text string) {
	state.UpdateUI(func() {
		if state.URLStatusLabel != nil {
			state.URLStatusLabel.SetText(text)
		}
	})
}

// SetProgress safely updates progress bar
func (state *WizardState) SetProgress(progress float64) {
	state.UpdateUI(func() {
		state.SetCheckURLState("", "", progress)
	})
}

// SetButtonText safely sets button text
func (state *WizardState) SetButtonText(button *widget.Button, text string) {
	state.UpdateUI(func() {
		if button != nil {
			button.SetText(text)
		}
	})
}
```

**Использование:**
```go
// Было (3 строки):
wizardstate.SafeFyneDo(state.Window, func() {
	state.URLStatusLabel.SetText("⏳ Checking...")
	state.SetCheckURLState("", "", 0.0)
})

// Стало (2 строки):
state.SetURLStatus("⏳ Checking...")
state.SetProgress(0.0)
```

**Экономия:** ~100 строк за счет упрощения вызовов.

**Приоритет:** СРЕДНИЙ

---

### 4. Консолидация валидации (экономия ~50 строк)

**Проблема:** Дублирование проверок длины строк в разных функциях валидации.

**Решение:** Объединить похожие функции в `wizard/business/validator.go`:

```go
// ValidateStringLength validates string length with min/max constraints
func ValidateStringLength(s, fieldName string, min, max int) error {
	if s == "" {
		return fmt.Errorf("%s is empty", fieldName)
	}
	if len(s) < min {
		return fmt.Errorf("%s length (%d) is less than minimum (%d)", fieldName, len(s), min)
	}
	if len(s) > max {
		return fmt.Errorf("%s length (%d) exceeds maximum (%d)", fieldName, len(s), max)
	}
	return nil
}

// ValidateURL использует общую функцию
func ValidateURL(urlStr string) error {
	if err := ValidateStringLength(urlStr, "URL", wizardutils.MinURILength, wizardutils.MaxURILength); err != nil {
		return err
	}
	// ... остальная логика парсинга URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}
	// ... остальные проверки
}
```

**Экономия:** ~50 строк за счет устранения дублирования.

**Приоритет:** СРЕДНИЙ

---

### 5. Упрощение тестов (экономия ~400 строк)

**Проблема:** Большие inline-структуры в table-driven тестах дублируются.

**Решение:** Вынести тестовые данные в отдельные функции:

```go
// В wizard/business/testdata.go (новый файл):
package business

import "singbox-launcher/core/config"

// ValidParserConfig returns a valid ParserConfig for testing
func ValidParserConfig() *config.ParserConfig {
	return &config.ParserConfig{
		ParserConfig: struct {
			Version   int                     `json:"version,omitempty"`
			Proxies   []config.ProxySource    `json:"proxies"`
			Outbounds []config.OutboundConfig `json:"outbounds"`
			Parser    struct {
				Reload      string `json:"reload,omitempty"`
				LastUpdated string `json:"last_updated,omitempty"`
			} `json:"parser,omitempty"`
		}{
			Version: 2,
			Proxies: []config.ProxySource{
				{
					Source:      "https://example.com/subscription",
					Connections: []string{"vless://uuid@server:443"},
				},
			},
			Outbounds: []config.OutboundConfig{
				{
					Tag:  "proxy-out",
					Type: "selector",
				},
			},
		},
	}
}

// InvalidParserConfig returns an invalid ParserConfig for testing
func InvalidParserConfig() *config.ParserConfig {
	return &config.ParserConfig{
		ParserConfig: struct {
			Version   int                     `json:"version,omitempty"`
			Proxies   []config.ProxySource    `json:"proxies"`
			Outbounds []config.OutboundConfig `json:"outbounds"`
			Parser    struct {
				Reload      string `json:"reload,omitempty"`
				LastUpdated string `json:"last_updated,omitempty"`
			} `json:"parser,omitempty"`
		}{
			Version: 1, // Invalid version
		},
	}
}
```

**Использование в тестах:**
```go
// В wizard/business/validator_test.go:
func TestValidateParserConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      func() *config.ParserConfig
		expectError bool
	}{
		{
			name:        "Valid ParserConfig",
			config:      ValidParserConfig,
			expectError: false,
		},
		{
			name:        "Invalid ParserConfig",
			config:      InvalidParserConfig,
			expectError: true,
		},
		// ...
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParserConfig(tt.config())
			if (err != nil) != tt.expectError {
				t.Errorf("ValidateParserConfig() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}
```

**Экономия:** ~400 строк за счет переиспользования тестовых данных.

**Приоритет:** НИЗКИЙ (тесты - это улучшение качества)

---

### 6. Упрощение обработки ошибок (экономия ~80 строк)

**Проблема:** Повторяющийся паттерн `fmt.Errorf("context: %w", err)`.

**Решение:** Создать helper-функции в `wizard/business/errors.go`:

```go
package business

import "fmt"

// WrapError wraps an error with context
func WrapError(err error, context string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(context, args...), err)
}

// ValidateAndWrap validates and wraps error if validation fails
func ValidateAndWrap(validate func() error, context string, args ...interface{}) error {
	if err := validate(); err != nil {
		return WrapError(err, context, args...)
	}
	return nil
}
```

**Использование:**
```go
// Было:
if err := ValidateURL(line); err != nil {
	return fmt.Errorf("proxy source %d: invalid URL: %w", i, err)
}

// Стало:
if err := ValidateAndWrap(
	func() error { return ValidateURL(line) },
	"proxy source %d: invalid URL", i,
); err != nil {
	return err
}
```

**Экономия:** ~80 строк за счет упрощения обработки ошибок.

**Приоритет:** НИЗКИЙ

---

### 7. Оптимизация комментариев (экономия ~100 строк)

**Проблема:** Избыточные комментарии для очевидных операций.

**Решение:** Убрать очевидные комментарии, оставить только важные:

```go
// Было:
// Split input into lines for processing
inputLines := strings.Split(input, "\n")

// Validate URL before fetching
if err := ValidateURL(line); err != nil {
	// ...
}

// Стало:
inputLines := strings.Split(input, "\n")

// Validate before network request to avoid unnecessary calls
if err := ValidateURL(line); err != nil {
	// ...
}
```

**Экономия:** ~100 строк за счет удаления избыточных комментариев.

**Приоритет:** НИЗКИЙ

---

## Итоговая экономия

| Оптимизация | Экономия строк | Приоритет | Сложность |
|-------------|----------------|-----------|-----------|
| Условное логирование | ~300 | ВЫСОКИЙ | Низкая |
| Утилита для таймингов | ~200 | СРЕДНИЙ | Средняя |
| Упрощение SafeFyneDo | ~100 | СРЕДНИЙ | Низкая |
| Упрощение тестов | ~400 | НИЗКИЙ | Средняя |
| Консолидация валидации | ~50 | СРЕДНИЙ | Низкая |
| Упрощение ошибок | ~80 | НИЗКИЙ | Низкая |
| Оптимизация комментариев | ~100 | НИЗКИЙ | Низкая |
| **Итого** | **~1,230 строк** | | |

## План внедрения

### Фаза 1: Быстрые победы (1-2 дня)
1. ✅ Условное логирование (~300 строк)
2. ✅ Упрощение SafeFyneDo (~100 строк)
3. ✅ Консолидация валидации (~50 строк)

**Экономия:** ~450 строк

### Фаза 2: Средние оптимизации (2-3 дня)
4. ✅ Утилита для таймингов (~200 строк)
5. ✅ Упрощение ошибок (~80 строк)

**Экономия:** ~280 строк

### Фаза 3: Опциональные улучшения (по необходимости)
6. ⚠️ Упрощение тестов (~400 строк) - можно отложить, т.к. тесты улучшают качество
7. ⚠️ Оптимизация комментариев (~100 строк) - можно отложить

**Экономия:** ~500 строк (опционально)

## Важные замечания

1. **Тесты не трогать без необходимости** - они улучшают качество кода, даже если занимают место
2. **Логирование в production** - обязательно сделать условным
3. **Не жертвовать читаемостью** - если оптимизация делает код сложнее, лучше не делать
4. **Измерять результаты** - после каждой фазы проверять реальную экономию строк

## Критерии успеха

- ✅ Условное логирование работает (можно отключить debug логи)
- ✅ Код компилируется без ошибок
- ✅ Все тесты проходят
- ✅ Функциональность не изменилась
- ✅ Читаемость кода не ухудшилась

## Примечания для агента

- Начинать с Фазы 1 (быстрые победы)
- После каждой оптимизации проверять компиляцию и тесты
- Не удалять тесты без крайней необходимости
- Сохранять обратную совместимость API
- Все изменения должны проходить `go vet` и `go test`

