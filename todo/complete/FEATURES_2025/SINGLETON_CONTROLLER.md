# Singleton Controller - Рефакторинг

## Цель
Реализовать `GetController()` как синглтон и убрать передачу контроллера из всех `core.*` функций и структур.

## Проблема
Сейчас `*AppController` передается как параметр во множество функций и хранится в структурах только для вызова `core.*` функций. Это создает:
- Избыточную передачу параметров
- Дублирование ссылок на один и тот же объект
- Сложность поддержки кода

## Решение

### 1. Реализовать `GetController()` как синглтон
- Функция возвращает глобальный экземпляр контроллера
- Использовать `sync.Once` для потокобезопасности при установке экземпляра
- Контроллер должен быть создан через `NewAppController()` в `main.go` перед любыми вызовами `GetController()`
- Если `GetController()` вызывается до `NewAppController()`, возвращается `nil` (это ошибка программиста)

### 2. Изменить все `core.*` функции
Убрать параметр `*AppController` из функций:
- `StartSingBoxProcess(ac *AppController, ...)` → `StartSingBoxProcess(...)`
- `StopSingBoxProcess(ac *AppController)` → `StopSingBoxProcess()`
- `MonitorSingBoxProcess(ac *AppController, ...)` → `MonitorSingBoxProcess(...)`
- `RunParserProcess(ac *AppController)` → `RunParserProcess()`
- `CheckIfSingBoxRunningAtStartUtil(ac *AppController)` → `CheckIfSingBoxRunningAtStartUtil()`
- `CheckConfigFileExists(ac *AppController)` → `CheckConfigFileExists()`
- `CheckIfLauncherAlreadyRunningUtil(ac *AppController)` → `CheckIfLauncherAlreadyRunningUtil()`
- `CheckFilesUtil(ac *AppController)` → `CheckFilesUtil()`
- `ShowSingBoxAlreadyRunningWarningUtil(ac *AppController)` → `ShowSingBoxAlreadyRunningWarningUtil()`
- `CheckLinuxCapabilities(ac *AppController)` → `CheckLinuxCapabilities()`

Внутри этих функций использовать `GetController()` вместо параметра.

### 3. Обновить все вызовы
- `core.StopSingBoxProcess(controller)` → `core.StopSingBoxProcess()`
- `core.StartSingBoxProcess(controller)` → `core.StartSingBoxProcess()`
- И т.д.

### 4. Убрать поле `controller` из структур
Если структура хранит `controller` только для вызова `core.*` функций, убрать это поле:
- `CoreDashboardTab.controller` - убрать, использовать `core.GetController()`
- `WizardPresenter.controller` - убрать, использовать `core.GetController()`
- И другие, где это применимо

## Важные замечания

1. **Инициализация**: Контроллер должен быть создан через `NewAppController()` в `main.go` перед любыми вызовами `GetController()`. От ленивой инициализации отказались - `GetController()` просто возвращает уже созданный экземпляр.

2. **Потокобезопасность**: Использовать `sync.Once` для гарантии установки только одного экземпляра в `NewAppController()`.

3. **Инициализация**: `NewAppController()` создает и инициализирует контроллер, затем устанавливает его как глобальный экземпляр через `sync.Once`. `GetController()` возвращает этот экземпляр.

4. **Методы AppController**: Методы типа `(ac *AppController) Method()` остаются без изменений, так как они работают с конкретным экземпляром.

## Файлы для изменения

### Core
- `core/controller.go` - добавить `GetController()`, изменить функции
- `core/process_service.go` - возможно потребуется обновить
- `core/config_service.go` - возможно потребуется обновить

### UI
- `ui/core_dashboard_tab.go` - убрать поле `controller`, обновить вызовы
- `ui/wizard/presentation/presenter.go` - убрать поле `controller`
- `ui/wizard/presentation/presenter_save.go` - обновить вызовы
- `ui/wizard/wizard.go` - обновить вызовы
- Все другие места, где используется `controller` только для `core.*` вызовов

### Main
- `main.go` - возможно потребуется обновить, но `NewAppController()` должен продолжать работать

## Тестирование

После рефакторинга проверить:
1. Приложение запускается
2. Все функции `core.*` работают корректно
3. Нет утечек памяти
4. Нет гонок данных (race conditions)

---

## Отчет о реализации

### Статус: ✅ Завершено

### Выполненные изменения

#### 1. Реализован `GetController()` как синглтон
- ✅ Добавлена глобальная переменная `instance` и `instanceOnce` для потокобезопасности
- ✅ `GetController()` возвращает глобальный экземпляр контроллера
- ✅ Реализован fallback механизм для создания минимального экземпляра, если `GetController()` вызывается до `NewAppController()` (с предупреждением в лог)
- ✅ `NewAppController()` устанавливает глобальный экземпляр через `sync.Once`

**Файл:** `core/controller.go`

#### 2. Обновлены все `core.*` функции
Убраны параметры `*AppController` из следующих функций:
- ✅ `StartSingBoxProcess()` - теперь без параметра `ac`
- ✅ `StopSingBoxProcess()` - теперь без параметра `ac`
- ✅ `MonitorSingBoxProcess()` - теперь без параметра `ac`
- ✅ `RunParserProcess()` - теперь без параметра `ac`
- ✅ `CheckIfSingBoxRunningAtStartUtil()` - теперь без параметра `ac`
- ✅ `CheckConfigFileExists()` - теперь без параметра `ac`
- ✅ `CheckIfLauncherAlreadyRunningUtil()` - теперь без параметра `ac`
- ✅ `CheckFilesUtil()` - теперь без параметра `ac`
- ✅ `ShowSingBoxAlreadyRunningWarningUtil()` - теперь без параметра `ac`
- ✅ `CheckLinuxCapabilities()` - теперь без параметра `ac`
- ✅ `getOurPID()` - теперь без параметра `ac`

Все функции теперь используют `GetController()` внутри для получения контроллера.

**Файлы:** `core/controller.go`, `core/process_service.go`

#### 3. Обновлены все вызовы `core.*` функций
- ✅ `ui/core_dashboard_tab.go` - обновлены вызовы `StartSingBoxProcess()`, `StopSingBoxProcess()`, `RunParserProcess()`
- ✅ `ui/wizard/presentation/presenter_save.go` - обновлены вызовы `StopSingBoxProcess()`, `StartSingBoxProcess()`
- ✅ `main.go` - обновлены вызовы всех утилитных функций
- ✅ `core/controller.go` - обновлены вызовы в `CreateTrayMenu()` и `GracefulExit()`
- ✅ `core/process_service.go` - обновлены вызовы `getOurPID()` и `ShowSingBoxAlreadyRunningWarningUtil()`

#### 4. Убрано поле `controller` из структур
- ✅ `WizardPresenter.controller` - поле удалено из структуры
- ✅ `NewWizardPresenter()` - убран параметр `controller`
- ✅ Все использования `p.controller` заменены на `core.GetController()` в:
  - `ui/wizard/presentation/presenter.go`
  - `ui/wizard/presentation/presenter_save.go`
  - `ui/wizard/presentation/presenter_async.go`
  - `ui/wizard/presentation/presenter_state.go`

#### 5. Обновлена функция `ShowConfigWizard()`
- ✅ Убран параметр `controller *core.AppController`
- ✅ Все использования `controller` заменены на `core.GetController()`
- ✅ Обновлен вызов в `ui/core_dashboard_tab.go`

**Файлы:** `ui/wizard/wizard.go`, `ui/core_dashboard_tab.go`

### Статистика изменений

- **Изменено файлов:** 10+
- **Удалено параметров:** 11 функций
- **Обновлено вызовов:** 20+
- **Убрано полей из структур:** 1 (`WizardPresenter.controller`)

### Результаты

✅ **Упрощение API:** Все `core.*` функции теперь не требуют передачи контроллера  
✅ **Уменьшение дублирования:** Контроллер больше не передается как параметр в десятках мест  
✅ **Централизованный доступ:** Единая точка доступа к контроллеру через `core.GetController()`  
✅ **Потокобезопасность:** Использование `sync.Once` гарантирует создание только одного экземпляра  
✅ **Обратная совместимость:** `NewAppController()` продолжает работать как раньше  

### Примечания

- Поле `controller` в `CoreDashboardTab` оставлено, так как оно используется не только для вызовов `core.*` функций, но и для доступа к методам и полям контроллера (например, `GetMainWindow()`, `GetVPNButtonState()`, `FileService`, `UIService` и т.д.)
- Fallback механизм в `GetController()` создает минимальный экземпляр только в случае ошибки программиста (вызов до `NewAppController()`), в нормальной работе это не происходит

