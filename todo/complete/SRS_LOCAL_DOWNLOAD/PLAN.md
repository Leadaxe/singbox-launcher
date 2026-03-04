# План: Локальное скачивание SRS

## 1. Архитектура

### 1.1 Компоненты

```
┌─────────────────────────────────────────────────────────────────┐
│  Rules Tab (UI)                                                 │
│  ├── Checkbox (rule) — Disable/Enable по наличию SRS            │
│  ├── Info button (?)                                            │
│  └── SRS button (⬇/🔄/✔️) — клик → DownloadSRS                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  SRSDownloader (или сервис)                                     │
│  DownloadSRS(ctx, url, destPath) error                           │
│  - HTTP GET, таймаут 60 с                                       │
│  - Сохранение в bin/rule-sets/{tag}.srs                         │
│  - context для отмены при закрытии визарда                      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  FileService / platform.EnsureDirectories                       │
│  - Создание bin/rule-sets/ при инициализации                    │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  MergeRouteSection (create_config.go)                            │
│  - Для rule_set с remote URL на raw.githubusercontent.com       │
│  - Подстановка type: "local", path: {ExecDir}/bin/rule-sets/{tag}.srs│
└─────────────────────────────────────────────────────────────────┘
```

### 1.2 Поток данных

1. **Загрузка визарда** → LoadTemplateData → rule_set с remote URL помечаются как «требующие SRS».
2. **При отрисовке Rules tab** → для каждого правила с SRS проверяем наличие файлов в `bin/rule-sets/` → устанавливаем состояние кнопки (⬇/✔️) и checkbox.Disable/Enable.
3. **Клик по ⬇ или ✔️** → DownloadSRS в goroutine → при успехе обновляем UI, при ошибке — диалог.
4. **При первом открытии** → для default-правил с SRS: если файлов нет → запускаем DownloadSRS в фоне → при успехе включаем правило.
5. **При сохранении** → MergeRouteSection подставляет `type: "local"`, `path` для SRS.

---

## 2. Изменения в файлах

### 2.1 wizard_template.json

- Удалить outbound `go-any-way-githubusercontent` из `parser_config.outbounds`.
- В `selectable_rules` для правил с remote SRS: удалить `download_detour`, `update_interval` из rule_set.

### 2.2 get_free.json

- Удалить outbound `go-any-way-githubusercontent` (если есть).

### 2.3 platform.EnsureDirectories / FileService

- Добавить создание `{ExecDir}/bin/rule-sets/` при инициализации.

### 2.4 Новый сервис или пакет

- `DownloadSRS(ctx context.Context, url string, destPath string) error`
- Размещение: `core/services/srs_downloader.go` или `internal/srs/download.go`

### 2.5 rules_tab.go

- `createSelectableRuleRowContent`: добавить кнопку SRS после иконки `?` для правил с SRS.
- Логика: проверка наличия SRS → `checkbox.Disable()`/`Enable()`, состояние кнопки.
- Обработчик клика по заблокированному чекбоксу → попап.
- Обработчик клика по кнопке ⬇/✔️ → вызов DownloadSRS.

### 2.6 create_config.go → MergeRouteSection

- При добавлении rule_set: если `type == "remote"` и `url` содержит `raw.githubusercontent.com` → заменить на `type: "local"`, `format: "binary"`, `path: "{ExecDir}/bin/rule-sets/{tag}.srs"`.

### 2.7 loader.go

- EnsureRequiredOutbounds: шаблон больше не содержит `go-any-way-githubusercontent` — ничего менять не нужно (outbound просто удалён из шаблона).

### 2.8 source_tab.go

- Удалить подсказку «do not change go-any-way-githubusercontent».

### 2.9 ParserConfig.md

- Удалить упоминания `go-any-way-githubusercontent`.

### 2.10 Формат path в конфиге

- Использовать **абсолютный путь**: `{ExecDir}/bin/rule-sets/{tag}.srs` — sing-box корректно обрабатывает абсолютные пути.

---

## 3. API DownloadSRS

**Сигнатура:** `DownloadSRS(ctx context.Context, url string, destPath string) error`

- Скачивает файл по HTTP(S) GET, сохраняет в destPath.
- Таймаут: 60 сек (или настраиваемый).
- При ошибке (сеть, 404, таймаут, диск full) возвращает error с понятным текстом.
- Конкурентность: один активный download на правило; повторный клик во время загрузки игнорируется.
- При `ctx.Done()` — прерывание, частичный файл удаляется или не используется.

---

## 4. Сводка изменений

| Действие | Элемент |
|----------|---------|
| **Удалить** | outbound `go-any-way-githubusercontent` из wizard_template.json, get_free.json |
| **Удалить** | `download_detour`, `update_interval` из rule_set в шаблоне (для SRS) |
| **Удалить** | подсказка «do not change go-any-way-githubusercontent» в source_tab.go |
| **Добавить** | директория `bin/rule-sets/` при инициализации |
| **Добавить** | функция `DownloadSRS(ctx, url, destPath)` |
| **Добавить** | кнопка ⬇/🔄/✔️ в rules_tab.go для правил с SRS |
| **Добавить** | логика `checkbox.Disable()`/`Enable()` по наличию SRS |
| **Изменить** | `MergeRouteSection` — подстановка `type: "local"`, `path` вместо remote |

---

## 5. DNS-правила для github

Правило `{"domain_suffix":["githubusercontent.com","github.com"],"server":"direct_dns_resolver"}` можно оставить (не мешает) или удалить — sing-box больше не обращается к raw.githubusercontent.com при работе.
