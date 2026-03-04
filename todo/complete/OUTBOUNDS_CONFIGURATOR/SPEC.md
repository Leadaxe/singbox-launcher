# SPEC: OUTBOUNDS_CONFIGURATOR — новая вкладочная структура визарда

**Реализация:** см. [IMPLEMENTATION_REPORT.md](IMPLEMENTATION_REPORT.md) — там описано, как всё сделано в коде (в т.ч. отличия от пунктов ниже).

---

## Проблема

Текущая вкладка **"Sources and ParserConfig"** в визарде совмещает сразу три разных задачи:
- ввод URL подписок и прямых ссылок (источники proxies),
- редактирование целого ParserConfig JSON,
- управление генерацией outbounds (Parse/Preview).

Это создаёт перегруженный UI, затрудняет понимание связей между источниками и outbounds, а также дублирует логику в отдельном окне **Config Outbounds**.

## Цель

Перестроить визард на две вкладки и встроенный конфигуратор outbounds так, чтобы:
- первая вкладка занималась только **источниками (Sources)** и быстрым просмотром результата парсинга;
- вторая вкладка занималась **outbounds и ParserConfig**;
- ParserConfig всегда был синхронизирован с действиями пользователя **без отдельной кнопки Parse**;
- пользователь не выходил из визарда в отдельное окно, а настраивал всё внутри одной многовкладочной формы.

## Общее поведение для пользователя

1. **Вкладка Sources**
   - Пользователь думает в терминах «подписки/ссылки → какие ноды получились».
   - Он вводит URL в многострочное поле и нажимает **Add** — URL добавляются к существующим источникам (дубликаты не добавляются). Видит:
     - подсказку по форматам URL и схемам;
     - список источников (по одному на каждый `ProxySource`) с коротким label, редактируемым `tag_prefix`, кнопками View и Del.
   - При наведении на элемент списка показывается tooltip с:
     - полным URL источника;
     - текущими `tag_prefix` / `tag_postfix` / `tag_mask` (если заданы);
     - количеством и списком тегов локальных outbounds этого источника.
   - Внизу вкладки — блок Preview: кнопка Refresh и список распарсенных серверов по всем источникам (тот же парсинг, что и для View по одному источнику).
   - На вкладке нет поля ParserConfig JSON и нет кнопки Parse; нет CheckURL/статуса проверки URL.

2. **Вкладка Outbounds** (в UI название вкладки — "Outbounds")
   - Пользователь управляет всеми outbounds в одном общем списке:
     - сначала локальные outbounds по каждому источнику;
     - затем глобальные outbounds из ParserConfig.
   - В каждой строке списка видны:
     - стрелки Up/Down для изменения порядка в пределах своего scope (локальные или глобальные outbounds);
     - tag и type;
     - метка источника (Global или label source);
     - кнопки Edit и Delete.
   - Кнопка Add открывает диалог создания/редактирования outbound.
   - В нижней/правой части вкладки расположен многострочный редактор ParserConfigJSON с кнопкой Documentation; кнопок Parse и ChatGPT на вкладке нет.

3. **Синхронизация ParserConfig**
   - Источник правды — структура `config.ParserConfig` в модели визарда.
   - Изменения состава/порядка outbounds в конфигураторе (и правки в списке Sources: prefix, Del) приводят к обновлению структуры ParserConfig и пересериализации. Ручной текст ParserConfigJSON применяется при уходе с вкладки Outbounds (валидация и откат при ошибке).
   - Списки Sources и Outbounds всегда пересчитываются из текущей структуры ParserConfig.

4. **Диалог Edit/Add Outbound**
   - Открывается из вкладки Outbounds как отдельное окно (аналогично диалогу добавления правил).
   - Позволяет задать:
     - scope (For all или For source: \<label\>);
     - tag, type (manual/auto), comment;
     - filters и preferredDefault с фиксированным ключом `tag`;
     - дополнительные outbounds (direct-out, reject и другие теги, расположенные выше в общем списке).
   - Весь контент диалога лежит внутри вертикального скролла с отступом под полосу прокрутки.

5. **Остальные вкладки визарда**
   - Вкладки Rules/Preview продолжают работать как сейчас, но всегда читают актуальный ParserConfig.
   - Parse/Preview триггерится автоматически на основе изменений ParserConfig и/или перехода на соответствующие вкладки.
   - Отдельного окна Config Outbounds больше нет — всё управление outbounds живёт во вкладке Outbounds and ParserConfig.

## Требования

### 1. Вкладки визарда

1. **Вкладка 1: Sources**
   - Содержимое:
     - многострочное поле SourceURLEntry (URL подписок + прямые ссылки);
     - кнопка Add справа от поля (применение только по Add, дубликаты не добавляются), подсказка под полем;
     - кнопка Get free VPN! в заголовке.
  - **Список Sources**:
     - компонент вида widget.List (или аналогичные контейнер + лейблы);
     - каждый элемент соответствует одному config.ProxySource из ParserConfig.ParserConfig.Proxies;
     - отображаемый текст: короткий label (обрезанный URL или Source 1, Source 2, ...), чтобы влезать по ширине;
     - при наведении на элемент показывается **tooltip** (через уже используемый в проекте пакет `github.com/dweymouth/fyne-tooltip`), содержащий:
       - полный source URL;
       - текущие `tag_prefix`, `tag_postfix`, `tag_mask` (если заданы);
       - количество локальных outbounds для этого источника и список их тегов.
   - **Preview**: нижний блок с кнопкой Refresh и списком распарсенных серверов по всем источникам (fetchAndParseSource по каждому ProxySource); список ограничен по высоте, справа полоса под скролл.
   - На вкладке **нет** поля ParserConfig JSON и **нет** кнопки Parse. В списке Sources: редактируемое поле tag_prefix, кнопки View (окно с серверами по ссылке) и Del.

2. **Вкладка 2: Outbounds and ParserConfig**
  - Верх/лево: **список outbounds** (конфигуратор):
     - порядок: сначала все локальные outbounds по каждому ProxySource (proxies[i].outbounds), затем глобальные (ParserConfig.outbounds);
     - каждая строка содержит:
       - иконки **↑** и **↓** (Fyne `theme.MoveUpIcon` / `theme.MoveDownIcon`) слева от текста;
       - текст `tag (type) — SourceLabel`, где SourceLabel = Global или короткий label источника (как в списке Sources);
       - справа — кнопки **Edit** и **Delete**;
     - кнопка **Up**:
       - доступна только если outbound не первый в своём scope;
       - меняет местами текущий outbound с предыдущим в том же слайсе:
         - локальные: внутри proxies[i].outbounds;
         - глобальные: внутри ParserConfig.outbounds;
     - кнопка **Down**:
       - доступна только если outbound не последний в своём scope;
       - меняет местами текущий outbound со следующим в том же слайсе;
     - кнопка **Add** открывает окно создания outbound; после каждой операции (Up/Down/Edit/Del/Add) вызывается onApply: сериализация ParserConfig, обновление модели и UI.
   - Редактор ParserConfig JSON выше списка; при уходе с вкладки вызывается ValidateAndApplyParserConfigFromEntry (применить или откатить ручные правки JSON). Кнопок Parse и ChatGPT нет.

### 2. Синхронизация ParserConfig

3. Единый источник правды — структура config.ParserConfig в модели визарда (model.ParserConfig).
4. ParserConfigJSON всегда является сериализацией этой структуры (через существующую нормализацию), а не независимым текстом.
5. **Обновление из вкладки Sources**:
   - применение URL только по кнопке Add через AppendURLsToParserConfig (добавление к существующим, без дубликатов); автоматического применения при вводе нет;
   - AppendURLToParserConfig обновляет model.ParserConfig (struct);
   - после успешного применения:
     - ParserConfig нормализуется и сериализуется в ParserConfigJSON;
     - редактор ParserConfig на вкладке Outbounds and ParserConfig получает новое значение (через SyncModelToGUI/UpdateParserConfig);
     - список Sources и outbounds пересчитывается из обновлённого ParserConfig.
6. **Обновление из вкладки Outbounds and ParserConfig (через UI)**:
   - Edit/Add/Delete/Up/Down в списке outbounds модифицируют model.ParserConfig (структуру: локальные и глобальные outbounds);
   - после каждой операции:
     - выполняется нормализация ParserConfig (version, reload, last_updated — по текущей логике);
     - структура сериализуется обратно в ParserConfigJSON;
     - редактор ParserConfig обновляется; Preview/Rules/Preview‑tab видят новые данные.
7. **Ручное редактирование ParserConfig JSON**:
   - пользователь может править текст в редакторе на вкладке Outbounds and ParserConfig;
   - при потере фокуса / по debounce / при переключении вкладки:
     - JSON валидируется и парсится в config.ParserConfig;
     - при успехе model.ParserConfig заменяется новой структурой, списки Sources/Outbounds пересчитываются;
     - при ошибке отображается понятное сообщение через dialogs.ShowError, редактор откатывается к последнему валидному ParserConfigJSON.

### 3. Диалог Edit/Add Outbound

8. Диалог редактирования/создания outbound открывается из вкладки Outbounds and ParserConfig и работает поверх неё (как сейчас окно configurator, но логически относится к вкладке).
 9. Поля диалога:
   - **Scope**: Select For all (глобальный outbound) или For source: <SourceLabel> (локальный для конкретного источника);
   - **Tag**: текстовое поле;
   - **Type**: Select `manual (selector)` / `auto (urltest)`;
   - **Comment**: текстовое поле (опционально);
   - **Filters**:
     - ключ зафиксирован как `tag` (лейбл, нередактируемый);
     - значение — строка‑паттерн (в т.ч. с отрицанием через !/regex/i и т.п.);
   - **Preferred default (preferredDefault)**:
     - ключ зафиксирован как `tag` (лейбл);
     - значение — строка‑паттерн для выбора узла по умолчанию (например, /🇳🇱/i);
   - **AddOutbounds**:
     - чекбоксы `direct-out`, `reject`;
     - чекбоксы по тегам других outbounds, которые находятся **выше** в текущем списке (локальные + глобальные) — чтобы зависимости были направлены только вниз.
10. Визуальные требования к диалогу:
    - контент вложен в вертикальный скролл, чтобы влезать по высоте окна;
    - справа внутри скролла зарезервирован прозрачный отступ (gap), чтобы полоса прокрутки не заезжала на поля ввода;
    - ширина формы фиксирована/минимальная, чтобы иконки и текст не ломались.

### 4. Список Sources и tooltip’ы

11. Список Sources на первой вкладке реализуется поверх уже существующей модели ParserConfig.ParserConfig.Proxies.
12. Для tooltip’ов используются компоненты из github.com/dweymouth/fyne-tooltip (см. уже реализованный PING_ERROR_TOOLTIP и раздел в ARCHITECTURE.md):
    - окно визарда уже обёрнуто в AddWindowToolTipLayer(content, canvas);
    - элементы списка/лейблы создаются как tooltip‑совместимые виджеты и получают SetToolTip(...) с текстом, описанным выше.
13. При изменении ParserConfig (URL, outbounds) tooltip‑данные должны обновляться автоматически при следующей отрисовке списка.

### 5. Поведение Preview и Rules

14. Логика генерации outbounds и Preview (ParseAndPreview, Rules‑tab, Preview‑tab) не меняется по сути, но опирается на обновлённый ParserConfig и ParserConfigJSON:
    - если раньше генерация привязывалась к кнопке Parse, теперь запуск Parse/Preview должен быть привязан к событиям изменения ParserConfig (URL, правки в Outbounds/JSON) и/или к переходу на Rules/Preview вкладки (как уже реализовано через TriggerParseForPreview/UpdateTemplatePreviewAsync).

## Критерии приёмки (реализовано)

- [x] Первая вкладка называется **Sources**, не содержит поля ParserConfig JSON и кнопки Parse; применение URL только по кнопке Add (AppendURLsToParserConfig, дубликаты не добавляются).
- [x] На вкладке Sources: список источников (по ProxySource) с редактируемым tag_prefix, View, Del; tooltip при наведении на label.
- [x] Вторая вкладка называется **Outbounds**; содержит редактор ParserConfig JSON (Documentation), список outbounds (↑/↓, Edit, Del), Add; кнопок Parse и ChatGPT нет.
- [x] Up/Down перемещают outbound только внутри своего scope; после операций конфигуратора вызывается onApply (сериализация, обновление модели и UI).
- [x] Конфигуратор встроен во вкладку Outbounds; отдельного окна Config Outbounds нет.
- [x] Изменения на Sources (Add, Del, prefix) и в конфигураторе Outbounds обновляют model.ParserConfig и ParserConfigJSON. Ручной JSON применяется при уходе с вкладки Outbounds (ValidateAndApplyParserConfigFromEntry).
- [x] Диалог Add/Edit — отдельное окно; поля Scope, Tag, Type, Comment, Filters (tag), Preferred default (tag), AddOutbounds (direct-out, reject, теги выше).
- [x] Rules/Preview используют актуальный ParserConfig.
- [x] UI на английском; ошибки через dialogs.ShowError, логирование через debuglog.
