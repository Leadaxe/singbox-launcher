package linkmap

// Вход ЭЛЕМЕНТА документа: разобранное значение JSON → пространство
// источников → исполнение плана.
//
// Отличие от входа ссылки не в «другом формате», а в уровне распаковки.
// Ссылка приезжает ТЕКСТОМ, и форма сама разворачивает оболочки (`decode`);
// элемент документа приезжает УЖЕ РАЗОБРАННЫМ — его достал уровень выше, тот
// же, что решает, какой элемент становится узлом, а какой остаётся хопом
// цепочки. Поэтому здесь нет ни конвейера декодеров, ни выбора формы по
// распакованному тексту: форму выбирает `detect` по типам контейнеров.
//
// ВИД ИСТОЧНИКА — ПАРАМЕТР, А НЕ ИМЯ В КОДЕ. Функции здесь общие для любого
// вида, чей элемент приезжает разобранным значением; какой именно вид
// разбирается, знает уровень документа — он же знает, какой ДОКУМЕНТ читает.
// Назвать вид в имени функции значило бы вернуть в движок второй, скрытый
// диспетчер диалекта — ровно то, от чего стоит страж TestNoSchemeNamesInEngine.

import (
	"fmt"

	"singbox-launcher/core/config/registry"
)

// SelectElementSection находит секцию ВИДА kind, чей detect опознаёт элемент.
//
// Пара к SelectURI, и правило неоднозначности у них ОДНО: две секции на один
// элемент — ошибка реестра, а не повод гадать. Движок отказывается вести
// такой элемент, и вызывающий получает «схема не поддержана».
//
// Имён схем здесь нет и быть не может: схему выбирает РЕЕСТР своим `detect`
// (`protocol` элемента), а не список в коде.
func SelectElementSection(plans *PlanSet, kind string, value interface{}) (string, *Plan, bool) {
	if plans == nil || kind == "" {
		return "", nil, false
	}
	content := NewElementContent(value, "", "")
	hit, plan := "", (*Plan)(nil)
	for _, scheme := range plans.Schemes() {
		p, ok := plans.Plan(scheme, kind)
		if !ok || p.Mapper == nil || p.Mapper.Detect == nil {
			continue
		}
		if !Matches(p.Mapper.Detect, content) {
			continue
		}
		if hit != "" {
			return "", nil, false
		}
		hit, plan = scheme, p
	}
	if hit == "" {
		return "", nil, false
	}
	return hit, plan, true
}

// ParseElement разбирает элемент документа планом схемы.
//
// Схему выбирает ВЫЗЫВАЮЩИЙ (SelectElementSection либо уровень документа):
// выбор плана — уровень документа, а не таблицы. Движок только исполняет.
func ParseElement(plan *Plan, value interface{}, bodyType string, trace *Trace) (*Result, error) {
	return ParseElementInDoc(plan, value, nil, bodyType, trace)
}

// ParseElementInDoc — то же для элемента, у которого есть СОСЕДИ по
// документу (контракт 1.1.63): записи с `deref` ищут среди doc элемент, на
// который ссылается значение, и читают его источниками `ref.<имя>.<путь>`.
// Какие элементы составляют документ, решает его уровень (массив outbounds
// Xray-конфига), а не таблица.
func ParseElementInDoc(plan *Plan, value interface{}, doc []interface{}, bodyType string, trace *Trace) (*Result, error) {
	space, form, err := UnwrapElement(plan, value)
	if err != nil {
		return nil, err
	}
	space.SetDocument(doc)
	return Exec(plan, space, form, bodyType, trace)
}

// UnwrapElement выбирает форму и кладёт элемент в пространство источников.
//
// Формы различает ТИП КОНТЕЙНЕРА, а не содержимое текста: мусорный тип
// (`"none"` вместо объекта, `[]` вместо объекта) означает БИТУЮ ЗАПИСЬ, а не
// «слоя нет». Не совпала ни одна форма — элемент узлом не становится, и это
// НЕ то же самое, что «разобрать как умеем»: ветка `default: true` собирала
// из битой записи рабочий узел без транспорта и TLS, которого провайдер не
// присылал.
//
// Ветка `default` здесь по-прежнему законна — просто её не объявляет ни одна
// живая секция этого вида: выбор ведёт общий Select, и правило «default не
// конкурирует с настоящим предикатом» действует то же, что на всех уровнях.
func UnwrapElement(plan *Plan, value interface{}) (*Space, registry.Form, error) {
	if plan == nil || plan.Mapper == nil {
		return nil, registry.Form{}, fmt.Errorf("linkmap: план не задан")
	}
	if len(plan.Mapper.Forms) == 0 {
		return nil, registry.Form{}, fmt.Errorf("linkmap: у секции нет форм")
	}

	content := NewElementContent(value, "", "")
	form, res := SelectForm(plan.Mapper, content)
	if res.Index < 0 {
		// Отказ ИМЕНОВАННЫЙ: элемент опознан как своя схема (detect секции
		// сошёлся), но ни одна форма его не приняла. Молчаливый nil на этом
		// месте выглядел бы как «схемы нет» и увёл бы разбирательство не туда.
		return nil, registry.Form{}, rejectUnrecognized(fmt.Errorf("linkmap: форма не распознана"))
	}

	space := &Space{}
	space.SetJSON(value)
	return space, form, nil
}
