// File source_body_edit.go — правка ТЕЛА узла-сервера из окна источника
// (SPEC 118 Т8).
//
// В модели v7 у server-узла одно тело (`body`) — готовый sing-box outbound, —
// и вкладка JSON правит именно его, а не «ручной config_json поверх URI»,
// как было до SPEC 118. Способов получить тело ровно два:
//
//   - Apply — пользователь вписал объект руками;
//   - Regen from raw — тело пересобирается из `origin.raw` (share-URI либо
//     исходный JSON), то есть из того, откуда узел изначально взялся.
//
// Оба идут через ЕДИНСТВЕННУЮ точку материализации (config.MaterializeServerNode)
// — ту же, что зовут fetch и миграция: вторая реализация разъехалась бы с
// первой на первой же правке эмиттера.
//
// Главное свойство обеих операций — ОТКАТ. Неразбираемый ввод оставляет узел
// ровно таким, каким он был: испортить рабочий узел неудачной попыткой его
// пересобрать нельзя. Поэтому обе функции ничего не мутируют до успеха и
// возвращают ошибку, а не пишут «пустое тело».
package tabs

import (
	"bytes"
	"encoding/json"
	"fmt"

	"singbox-launcher/core/config"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// applyServerBodyJSON — Apply вкладки JSON: текст → тело узла.
//
// json.Compact (а не Unmarshal→Marshal) сохраняет порядок ключей, который
// написал пользователь: пересортировка по алфавиту меняла бы тело на каждом
// сохранении и ломала байт-сравнение с эмиссией.
//
// Возвращает ошибку и НЕ трогает узел, если текст не JSON, не объект или без
// непустого `type` — ядро такой outbound не принимает, а принять его молча
// значило бы сломать весь конфиг ради одного узла.
//
// Аргумент — УЗЕЛ, а не Source: путь правки тела один на верхний узел
// (`&scratch.Node` в окне источника) и на узел контейнера (строка папки или
// подписки, W13 заход 2). Ни `Body`, ни `Origin` к Source-обвязке отношения
// не имеют — они поля `Node`, и сужение сигнатуры до них не даёт завести
// вторую реализацию «текст → тело» для узла папки (ловушка «эмиттер и парсер
// ходят парой»).
//
// ownContainer — узел свой: свободный сервер в корне или член папки, не узел
// подписки. Условие контейнера знает только вызывающий (адрес окна).
func applyServerBodyJSON(node *wizardmodels.Node, text string, ownContainer bool) error {
	if node == nil {
		return fmt.Errorf("no node")
	}
	bodyText, err := nodeBodyFromJSONInput(text)
	if err != nil {
		return err
	}
	// Свой узел после ручной правки JSON — АВТОРСКИЙ (контракт 1.1.88,
	// PARSING_PRINCIPLES §11 п.1; решение владельца 27.09.2026, LxBox §576):
	// в источнике записи остаётся только голое тело, вид `json`, какой бы ни
	// была прежняя форма — ссылка, INI, документ. Реестр на таком теле
	// сообщает, а не правит (§10). Предупреждение о потере ссылки показывает
	// вызывающий ДО вызова (ownEditDropsOrigin).
	//
	// Узел подписки — не свой: вид и исходник происхождения переживают правку
	// тела, и правила реестра применяются как у обычного тела.
	materialize := config.MaterializeServerNode
	if !ownContainer && node.Origin != nil && node.Origin.Kind != wizardmodels.OriginKindJSON {
		materialize = func(_ string, js json.RawMessage) (*config.ServerNodeMaterial, error) {
			return config.MaterializeEditedBody(js)
		}
	}
	mat, err := materialize("", json.RawMessage(bodyText))
	if err != nil {
		return err
	}
	bodyBefore := node.Body
	node.Body = mat.Body
	// Коды пересчитаны по НОВОМУ телу и замещают прежние целиком (Л5):
	// после правки тела старый набор описывает узел, которого больше нет.
	// Вердикт ядра — не код разбора, пересчётом он не снимается.
	node.ReplaceDerivedWarnings(mat.Warnings)
	// А вот СМЕНА ТЕЛА его снимает (SPEC 132, PARSING_PRINCIPLES §9.4): ядро судило о
	// прежнем теле, и к новому его приговор неприменим — узел включается
	// обратно и проверится следующей сборкой.
	node.RevalidateCoreVerdictAfterBodyChange(bodyBefore)

	// СВЯЗЬ С ПОДПИСКОЙ ручная правка рвёт (Д5, разыменование): тело
	// теперь наше, и следующий fetch не имеет права его переписать. Поэтому
	// origin.SubURL снимается — это ровно то, что делает
	// business.DereferenceNodeOrigin. Origin ПЕРЕСАЖИВАЕТСЯ на новый
	// экземпляр: Node несёт *Origin, и копии узла делят его с оригиналом.
	if node.Origin == nil || ownContainer {
		// Узел без происхождения собран прямо здесь из вставленного JSON, а
		// свой узел после правки происходит из неё: источник — голое тело.
		node.Origin = &wizardmodels.Origin{Kind: wizardmodels.OriginKindJSON, Raw: mat.OriginRaw}
		return nil
	}
	// Узел подписки: ВИД и ИСХОДНИК происхождения переживают правку тела.
	// Узел, заведённый из wg-quick INI, иначе терял бы исходник (блок
	// [Interface]/[Peer] с комментариями), и «Regen from raw» пересобирал бы
	// его уже из нашего собственного вывода.
	if node.Origin.Kind == wizardmodels.OriginKindJSON && node.Origin.Raw != mat.OriginRaw {
		// Источник JSON-узла после правки — только тело узла (контракт
		// 1.1.87, PARSING_PRINCIPLES §11 п.1): документ из вкладки в источник
		// не попадает.
		o := *node.Origin
		o.Raw = mat.OriginRaw
		node.Origin = &o
	}
	if node.Origin.SubURL != "" {
		o := *node.Origin
		o.SubURL = ""
		node.Origin = &o
	}
	return nil
}

// ownEditDropsOrigin — заменит ли правка JSON своего узла его исходник
// ссылку или INI (ownContainer: свободный сервер в корне или член папки).
// Вызывающий спрашивает подтверждение до applyServerBodyJSON: узел
// перестаёт быть связан со ссылкой, и реестр его больше не правит.
func ownEditDropsOrigin(node *wizardmodels.Node, ownContainer bool) bool {
	return ownContainer && node != nil && node.Origin != nil &&
		node.Origin.Kind != wizardmodels.OriginKindJSON && node.Origin.Raw != ""
}

// nodeBodyFromJSONInput — ввод вкладки JSON → текст тела узла. Формы ввода
// три: голое тело, документ узла и массив тел (PARSING_PRINCIPLES §11 п.1);
// в хранение уходит только тело. Об остатке документа или массива сообщает
// вызывающий (jsonInputDropsRest) — одним сообщением на сохранение.
func nodeBodyFromJSONInput(text string) (string, error) {
	if config.IsNodeDocument([]byte(text)) {
		body, _, err := config.ParseNodeDocument([]byte(text))
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	if config.IsNodeBodyArray([]byte(text)) {
		body, _, err := config.ParseNodeBodyArray([]byte(text))
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	var ob map[string]interface{}
	if err := json.Unmarshal([]byte(text), &ob); err != nil {
		return "", err
	}
	if t, _ := ob["type"].(string); t == "" {
		return "", fmt.Errorf("outbound object must have a non-empty \"type\" field")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(text)); err != nil {
		return "", err
	}
	return compact.String(), nil
}

// jsonInputDropsRest — осталось ли во вводе вкладки JSON что-то кроме тела
// узла: `dns`/`route`/`sections` документа или элементы массива после
// первого.
func jsonInputDropsRest(text string) bool {
	if config.IsNodeDocument([]byte(text)) {
		_, dropped, err := config.ParseNodeDocument([]byte(text))
		return err == nil && len(dropped) > 0
	}
	if config.IsNodeBodyArray([]byte(text)) {
		_, extra, err := config.ParseNodeBodyArray([]byte(text))
		return err == nil && extra > 0
	}
	return false
}

// regenServerBodyFromRaw — «Regen from raw»: тело пересобирается из
// происхождения узла.
//
// Ветка выбирается по виду происхождения: JSON-узел разбирается как объект,
// всё остальное — как share-URI. Ошибка = ОТКАТ: узел остаётся прежним, и
// вызывающий показывает причину. Именно ради этого материализация идёт во
// временные переменные, а не прямо в поля источника.
func regenServerBodyFromRaw(node *wizardmodels.Node) error {
	if node == nil || node.Origin == nil {
		return fmt.Errorf("no origin to regenerate from")
	}
	return regenServerBodyFromRawText(node, node.Origin.Raw)
}

// regenServerBodyFromRawText — тот же Regen, но из ЯВНО переданного текста
// происхождения (SPEC 119, фаза 2).
//
// Нужен правке raw в окне узла: модель разрешает править исходник, но
// действует правка только через Regen (features/sources.md, §Происхождение).
// Пересборка идёт из текста, который пользователь видит в поле, а не из
// сохранённого в узле, — иначе кнопка Regen применяла бы прежний исходник и
// правка молча пропадала бы.
//
// Успех перезаписывает origin.raw новым текстом: он и есть новое
// происхождение узла. Ошибка — ОТКАТ, узел остаётся прежним.
func regenServerBodyFromRawText(node *wizardmodels.Node, raw string) error {
	if node == nil || node.Origin == nil {
		return fmt.Errorf("no origin to regenerate from")
	}
	if raw == "" {
		return fmt.Errorf("origin is empty")
	}
	var (
		mat *config.ServerNodeMaterial
		err error
	)
	if node.Origin.Kind == wizardmodels.OriginKindJSON {
		mat, err = config.MaterializeServerNode("", json.RawMessage(raw))
	} else {
		mat, err = config.MaterializeServerNode(raw, nil)
	}
	if err != nil {
		return err
	}
	bodyBefore := node.Body
	node.Body = mat.Body
	// Regen пересобирает узел из исходника — значит и коды считаются заново
	// и замещают прежние (Л5, §3.5 «warnings замещаются, не дописываются»).
	// Вердикт ядра — не код разбора, пересчётом он не снимается; его снимает
	// СМЕНА ТЕЛА (SPEC 132, PARSING_PRINCIPLES §9.4).
	node.ReplaceDerivedWarnings(mat.Warnings)
	node.RevalidateCoreVerdictAfterBodyChange(bodyBefore)
	node.Origin = &wizardmodels.Origin{Kind: mat.OriginKind, Raw: mat.OriginRaw}
	return nil
}
