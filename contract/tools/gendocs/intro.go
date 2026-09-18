package main

import "strings"

// Вводный блок «How to read this page».
//
// Отзыв владельца: читатель не понимал, чем «Link parameters» отличаются от
// «Body fields», и считал, что раздел «Link parameters» — это весь словарь
// ссылки. Объяснение конвейера вынесено в начало каждой страницы и
// сопровождается маленькой схемой: словарь ссылки → маппер → тело узла →
// санитайзер → конфиг ядра. Без этой схемы названия разделов приходится
// угадывать.
//
// Блок адаптируется: у схемы без ссылочной формы (chain, socks, tailscale)
// раздела «Link parameters» на странице нет, и упоминать его нельзя — иначе
// вводный текст отправляет читателя в несуществующий раздел.

// pipelineDiagram — схема конвейера в code-блоке. Ступени у всех схем одни и
// те же, меняется только то, есть ли у схемы вход со стороны ссылки; сама
// ссылка в примере — той схемы, чья это страница.
func pipelineDiagram(scheme string) string {
	if scheme == "" {
		return "```\n" +
			"sing-box JSON       node body              sanitizer            core config\n" +
			"or a form field ──▶ sing-box JSON,    ──▶  checks each     ──▶  what sing-box\n" +
			"                    stored in state         body field's         is actually\n" +
			"                                            value                started with\n" +
			"```\n\n"
	}
	// Пример ссылки выравнивается по колонке «share link»: схемы разной длины,
	// и без добивки стрелка уезжает относительно остальных ступеней. Ширина
	// считается в рунах — многоточие и знак вопроса тут не ASCII.
	const col = 20
	example := scheme + "://…?…"
	if n := len([]rune(example)); n < col {
		example += strings.Repeat(" ", col-n)
	}
	return "```\n" +
		"share link          mapper                node body              sanitizer            core config\n" +
		example + "──▶ link parameter   ──▶  sing-box JSON,    ──▶  checks each     ──▶  what sing-box\n" +
		"                     becomes a body         stored in state        body field's         is actually\n" +
		"                     field                                         value                started with\n" +
		"```\n\n"
}

// schemeIntro — «How to read this page» для страницы схемы. shared говорит,
// есть ли у схемы общие параметры (TLS/транспорт): у wireguard их нет, и
// обещать их читателю нельзя.
func schemeIntro(scheme string, withLink, shared bool) string {
	var b strings.Builder
	b.WriteString("## How to read this page\n\n")
	if withLink {
		b.WriteString(pipelineDiagram(scheme))
	} else {
		b.WriteString(pipelineDiagram(""))
	}

	if withLink {
		line := "- **Link parameters** — the dictionary of the share link: every parameter " +
			"this scheme understands, and the body field each one becomes."
		if shared {
			line += " The TLS and transport parameters shared with other schemes are listed " +
				"here too, not only on their reference pages."
		}
		b.WriteString(line + "\n")
	}
	if withLink {
		b.WriteString("- **Body fields** — the node body itself: the sing-box JSON kept in the " +
			"launcher state. The rules here hold for every input alike — a share link, " +
			"sing-box JSON, Xray JSON or a hand-filled form — because they are checked after " +
			"the input has already become a body.\n")
	} else {
		b.WriteString("- **Body fields** — the node body itself: the sing-box JSON kept in the " +
			"launcher state, with the rule applied to each value.\n")
	}
	b.WriteString("- **Diagnosed problems** — every warning code a node of this scheme can carry, " +
		"and the field that raises it.\n")
	b.WriteString("- **Replacements** — what is silently rewritten on the way in: other spellings " +
		"of the same name, values normalized or substituted, and structural decisions the " +
		"mapper takes before any value is judged.\n")
	b.WriteString("- **Degradation** — the same rules grouped by outcome: what drops the node, " +
		"what only drops a field, and what is merely worth knowing.\n\n")

	if withLink {
		b.WriteString("A bad value never breaks the whole config: the field is dropped, replaced " +
			"or — at worst — the single node is. Each link parameter says which of the three " +
			"happens to it, taken from the rule of the body field it maps to.\n\n")
	} else {
		b.WriteString("A bad value never breaks the whole config: the field is dropped, replaced " +
			"or — at worst — the single node is.\n\n")
	}
	return b.String()
}

// indexIntro — короткая версия того же объяснения для оглавления.
func indexIntro() string {
	var b strings.Builder
	b.WriteString("## How to read these pages\n\n")
	b.WriteString(pipelineDiagram("vless"))
	b.WriteString("Each scheme page lists the **link parameters** (the share-link dictionary and " +
		"the body field every parameter becomes) and the **body fields** (the sing-box JSON kept " +
		"in the launcher state, with the rule applied to each value). The body rules hold for " +
		"every input alike — link, sing-box JSON, Xray JSON or a hand-filled form. Three more " +
		"sections follow: the warning codes a node can carry, what is silently replaced on the " +
		"way in, and what a bad value costs — a field or the whole node.\n\n")
	return b.String()
}
