// File folder_fill.go — наполнение папки узлами из текста (SPEC 116 этап 3,
// W6; требование цели 2, сценарий С1).
//
// Второй адрес назначения для ТОГО ЖЕ разбора, что кормит корневой Add:
// `parseSourceInput` (source_input.go) даёт узлы, а здесь они кладутся в
// `Source.Nodes` папки вместо `model.Sources`. Своего разбора тут нет и быть
// не может — см. шапку source_input.go.
//
// # Чем это отличается от корневой ветки
//
//   - подписка узлом не становится: контейнер внутрь контейнера не кладут
//     (вложенных папок нет — features/sources.md). Подписочная строка во
//     входе папки — не молчаливый пропуск, а названная ошибка;
//   - уникализация тега считается в пределах ЭТОЙ папки, а не корня: сырой
//     тег — идентичность узла внутри контейнера (SPEC 112), и два одинаковых
//     там невозможны. У корня дедуп другой — по исходному URI;
//   - `Source.ID` узлам не выдаётся: ULID есть только у Source, а узел папки
//     адресуется парой (FolderID, сырой тег).
//
// Побочки (BumpRevision / InvalidateNodePool / MarkAsChanged) — на вызывающем
// UI, как у node_move.go: одна пользовательская операция (мультивыбор файлов)
// зовёт эту функцию по разу на файл, а ревизия обязана дёрнуться один раз.
package business

import (
	"errors"
	"fmt"
	"strings"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// AppendNodesToFolder разбирает `input` и кладёт полученные узлы в хвост
// `Source.Nodes` папки `folderID`.
//
// `defaultTag` — имя для узлов, у которых своего имени во входе не было (нет
// `#fragment` у ссылки, нет `tag` у JSON-объекта). Правило
// features/sources.md §«Наполнение папки» п.1: при импорте файлов таким
// именем становится имя файла. Пусто — остаётся заглушка `server-N`.
// Несколько безымянных узлов из одного входа получают `defaultTag`, `-2`,
// `-3`… той же уникализацией, что и остальные: имя файла одно, а узлов в нём
// может быть много.
//
// В хвост, а не сортировкой: порядок узлов папки принадлежит пользователю
// (features/sources.md §«Наполнение папки»), и приехавший узел не вправе
// раздвигать чужие позиции — ровно как у placeNodeIntoFolder.
//
// Возвращает результат: сколько узлов легло и сколько подписочных строк
// пришлось отбросить. Отброшенные СЧИТАЮТСЯ, а не молчат: смешанный вход
// (пара ссылок и подписка одним куском) — законная вставка, ронять её целиком
// незачем, но и терять строку молча нельзя. Когда узлов не получилось вовсе,
// а подписка была, это уже не предупреждение, а ошибка операции —
// `ErrSubscriptionInFolder`.
func AppendNodesToFolder(m *wizardmodels.WizardModel, folderID, input string, defaultTag string) (FolderFillResult, error) {
	var res FolderFillResult
	if m == nil {
		return res, fmt.Errorf("add nodes: model is nil")
	}
	idx, err := lookupFolderIndex(m, folderID)
	if err != nil {
		return res, err
	}
	folder := &m.Sources[idx]

	parsed, err := parseSourceInput(strings.TrimSpace(input), len(folder.Nodes))
	if err != nil {
		return res, err
	}
	// Подписка в папку не кладётся ни узлом, ни вложенным контейнером.
	// Сказать об этом обязательно: пользователь вставил строку и вправе знать,
	// почему она не превратилась ни во что.
	if len(parsed.Nodes) == 0 && len(parsed.Subscriptions) > 0 {
		return res, ErrSubscriptionInFolder
	}
	res.SkippedSubscriptions = len(parsed.Subscriptions)

	taken := make(map[string]bool, len(folder.Nodes)+len(parsed.Nodes))
	for i := range folder.Nodes {
		taken[folder.Nodes[i].Tag] = true
	}

	for i := range parsed.Nodes {
		node := parsed.Nodes[i]
		// Имя из внешнего источника (имя файла) — только безымянному узлу:
		// собственный `#fragment` пользователь написал сам, и подменять его
		// именем файла значило бы терять его выбор.
		if defaultTag != "" && i < len(parsed.Unnamed) && parsed.Unnamed[i] {
			node.Tag = defaultTag
		}
		node.Tag = uniqueTagIn(taken, node.Tag)
		if node.Tag == "" {
			continue
		}
		taken[node.Tag] = true
		folder.Nodes = append(folder.Nodes, node)
		res.Added++
	}

	if res.Added == 0 && res.SkippedSubscriptions > 0 {
		return FolderFillResult{}, ErrSubscriptionInFolder
	}
	return res, nil
}

// FolderFillResult — исход одного наполнения папки.
type FolderFillResult struct {
	// Added — сколько узлов легло в папку.
	Added int
	// SkippedSubscriptions — сколько подписочных строк во входе пришлось
	// отбросить: вложенных контейнеров нет, и подписка узлом не становится.
	SkippedSubscriptions int
}

// ErrSubscriptionInFolder — во входе была подписка, а узлов не получилось.
//
// Сентинел, а не форматная строка: UI обязан ПОДМЕНИТЬ этот случай своим
// переведённым текстом (business пользовательских строк не переводит), а
// сравнение по подстроке сообщения ломается от любой правки формулировки —
// молча, без единой ошибки компиляции.
var ErrSubscriptionInFolder = errors.New(
	"a subscription URL cannot be added to a folder — add it in Sources, then fill the folder from it")

// TagFromFileName — тег узла из имени импортируемого файла
// (features/sources.md §«Наполнение папки» п.1: «узлу без собственного имени
// во фрагменте тег даётся из имени файла»).
//
// Расширение отбрасывается: `Netherlands.conf` человек называет
// «Netherlands», и тег `Netherlands.conf` в списке outbound'ов выглядел бы
// как чужая внутренность. Разделители пути срезаются на всякий случай —
// нативные диалоги отдают полный путь, а тег с `/` внутри сломал бы фильтры
// Направлений.
func TagFromFileName(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	if i := strings.LastIndex(p, "."); i > 0 {
		p = p[:i]
	}
	return strings.TrimSpace(p)
}
