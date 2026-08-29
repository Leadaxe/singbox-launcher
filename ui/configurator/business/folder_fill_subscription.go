// File folder_fill_subscription.go — заливка подписки в папку (SPEC 116 этап 3,
// W7; цель П4, сценарий С5, критерий A5).
//
// # Материал заливки — уже материализованные nodes[] подписки
//
// Не повторный fetch и не разбор тела: тело подписки в модели не хранится
// вовсе (features/state.md §«Чего не хранится»), а единственный разбор живёт в
// fetch-конвейере (`config.MaterializeSubscriptionBody`). Заливка берёт ровно
// то, что этот конвейер уже положил в `Source.Nodes` подписки — иначе в
// лаунчере появился бы второй разбор тела, и он неминуемо разъехался бы с
// первым (ловушка «эмиттер и парсер ходят парой», она же «сборка не парсит
// тела подписок»).
//
// Отсюда же ответ на «а если подписку ни разу не обновляли»: материала нет,
// добывать его здесь нечем — заливка отказывается и зовёт обновить подписку
// штатным путём (ErrSubscriptionNotFetched).
//
// # Почему копия узлов, а не сами узлы
//
// `SubFetchMaterial.Nodes` merge читает как чужой материал и делает
// поверхностные копии (см. шапку subscription_merge.go). Поверхностной копии
// достаточно ровно потому, что merge пересаживает Origin и Group на клоны, а
// не правит на месте. Здесь тот же контракт соблюдён с другой стороны: узлы
// подписки отдаются как есть, и ни одно поле по указателю заливкой не
// правится — иначе заливка в папку отвязала бы от провайдера саму подписку.
//
// # Truncated
//
// Признак берётся из `sub.UpdateStatus.Truncated` — состояния последнего
// разбора, а не из длины nodes[]: «упёрлись в кап» знает только fetch, и
// вычислить это по составу нельзя. При truncated merge не разыменовывает
// исчезнувших (SPEC 113-A: «исчез» неотличим от «остался за капом»).
package business

import (
	"errors"
	"fmt"
	"strings"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// ErrSubscriptionNotFetched — у подписки нет материализованных узлов: её ещё
// ни разу не обновляли (или последнее обновление не удалось).
//
// Сентинел, как ErrSubscriptionInFolder: UI обязан подменить его переведённым
// текстом И предложить обновление, а сравнение по подстроке сообщения молча
// сломалось бы от правки формулировки.
var ErrSubscriptionNotFetched = errors.New(
	"subscription has no nodes yet — refresh it first, then fill the folder from it")

// FolderSubscriptionFillResult — исход одной заливки.
type FolderSubscriptionFillResult struct {
	// Changed — состав/содержимое nodes[] папки реально изменились.
	Changed bool
	// Warnings — деградации merge как их сформулировало ядро (исчез у
	// провайдера, занятый тег, prune члена группы). Пользовательскими строками
	// они не являются и через locale не идут: это диагностика, и её формат
	// принадлежит ядру.
	Warnings []string
	// SubName — как звали подписку на момент заливки: сообщение об исходе
	// показывается уже после мутации, а имя к тому времени искать негде.
	SubName string
}

// FillFolderFromSubscription заливает узлы подписки subID в папку folderID
// (сценарий С5). Повторная заливка идёт этим же путём — отдельного «обновить
// папку» не существует: merge идемпотентен, и второй вызов на том же материале
// не меняет ничего (folder_merge_test.go).
//
// Побочки (BumpRevision / InvalidateNodePool / MarkAsChanged) — на вызывающем
// UI, как у AppendNodesToFolder и node_move.go.
func FillFolderFromSubscription(m *wizardmodels.WizardModel, folderID, subID string) (FolderSubscriptionFillResult, error) {
	var res FolderSubscriptionFillResult
	if m == nil {
		return res, fmt.Errorf("fill folder: model is nil")
	}
	folderIdx, err := lookupFolderIndex(m, folderID)
	if err != nil {
		return res, err
	}
	subIdx, err := lookupSubscriptionIndex(m, subID)
	if err != nil {
		return res, err
	}
	sub := &m.Sources[subIdx]
	res.SubName = SourceDisplayName(*sub)

	subURL := strings.TrimSpace(sub.URL)
	if subURL == "" {
		// Ключ заливки — пара (origin.subUrl, сырой тег). Подписка без URL
		// адресовать свои копии не может, и merge на пустом url — no-op:
		// сказать об этом честнее, чем молча ничего не сделать.
		return res, fmt.Errorf("fill folder: subscription %q has no URL", res.SubName)
	}
	if len(sub.Nodes) == 0 {
		return res, ErrSubscriptionNotFetched
	}

	// Глубокая копия материала. Merge сам по указателям не пишет (Origin и
	// Group он пересаживает на клоны), но узел папки после заливки продолжал бы
	// ДЕЛИТЬ Body и Hops с живым узлом подписки, а это ровно тот класс связей,
	// который однажды сработает в обратную сторону. Копия рвёт её здесь, на
	// границе двух источников, тем же клоном, что и перенос узла.
	material := &corestate.SubFetchMaterial{
		Nodes:     make([]corestate.Node, 0, len(sub.Nodes)),
		Truncated: sub.UpdateStatus != nil && sub.UpdateStatus.Truncated,
	}
	for i := range sub.Nodes {
		// Неразобранные записи (kind=unsupported, SPEC 116 W11) в папку не
		// едут: папка — рабочий состав пользователя, а такая запись это
		// диагностика подписки, и чинит её провайдер, не заливка. В подписке
		// она видна и без копии.
		if sub.Nodes[i].IsUnsupported() {
			continue
		}
		material.Nodes = append(material.Nodes, cloneCanonicalNodeForMove(sub.Nodes[i]))
	}
	if len(material.Nodes) == 0 {
		// Состав есть, но собравшихся узлов в нём нет — одни неразобранные
		// записи. Лить нечего, и пустой merge разыменовал бы все прежние копии
		// как «исчезнувшие у провайдера»: отказ честнее. Предложение обновить
		// подписку остаётся верным — чинится это её обновлением.
		return res, ErrSubscriptionNotFetched
	}
	// trusted=true: материал взят не из сети, а из состояния — он по
	// построению достоверен (недостоверный ответ до nodes[] не доезжает,
	// MergeSubscriptionNodes его отбрасывает). Гард «недостоверного ответа»
	// закрыт выше проверкой len(sub.Nodes) == 0.
	changed, warns := corestate.MergeFolderNodesFromSubscription(
		&m.Sources[folderIdx], subURL, material, true)
	res.Changed = changed
	res.Warnings = warns
	return res, nil
}

// FolderFillSubscriptionChoice — одна строка списка «из какой подписки лить».
type FolderFillSubscriptionChoice struct {
	ID string
	// Name — как подписку зовут пользователю (SourceDisplayName).
	Name string
	// URL — вторая строка выбора: имена подписок не уникальны, а URL и есть
	// ключ заливки, поэтому именно он различает две одноимённые.
	URL string
	// NodeCount — сколько узлов подписка отдаст. Ноль означает «ни разу не
	// обновляли»: строку показываем, но заливка по ней откажет с
	// ErrSubscriptionNotFetched — прятать её было бы хуже, пользователь искал
	// бы пропавшую подписку.
	NodeCount int
}

// FolderFillSubscriptions — подписки модели в порядке списка Sources.
//
// Порядок исходный, без сортировки: человек ищет подписку глазами там же, где
// она лежит в списке источников.
func FolderFillSubscriptions(m *wizardmodels.WizardModel) []FolderFillSubscriptionChoice {
	if m == nil {
		return nil
	}
	var out []FolderFillSubscriptionChoice
	for i := range m.Sources {
		s := &m.Sources[i]
		if s.Kind != corestate.SourceKindSubscription {
			continue
		}
		id := strings.TrimSpace(s.ID)
		if id == "" {
			continue
		}
		out = append(out, FolderFillSubscriptionChoice{
			ID:        id,
			Name:      SourceDisplayName(*s),
			URL:       strings.TrimSpace(s.URL),
			NodeCount: len(s.Nodes),
		})
	}
	return out
}

// lookupSubscriptionIndex — позиция подписки по ULID.
//
// Отдельно от lookupFolderIndex, а не общей функцией с параметром kind: у этих
// двух разные сообщения об ошибке, и «папка не найдена» вместо «подписка не
// найдена» отправило бы искать не туда.
func lookupSubscriptionIndex(m *wizardmodels.WizardModel, subID string) (int, error) {
	id := strings.TrimSpace(subID)
	if id == "" {
		return -1, fmt.Errorf("fill folder: subscription id is empty")
	}
	for i := range m.Sources {
		if m.Sources[i].Kind == corestate.SourceKindSubscription && strings.TrimSpace(m.Sources[i].ID) == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("fill folder: subscription %q not found", id)
}
