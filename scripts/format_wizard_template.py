#!/usr/bin/env python3
"""Приводит bin/wizard_template.json к editorial-стилю docs/TEMPLATE_REFERENCE §10.

Зачем: порядок ключей и переносы на loader не влияют (§10 — readability, не
контракт), но 45 КБ машинного `indent=2` невозможно ревьюить. Скрипт держит
единый вид: компактно — литералы и metadata, развёрнуто — выражения (`@…`,
`#if`, outer `if[]`) и длинные списки.

Портирован из LxBox (app/scripts/format_wizard_template.py) под схему
лаунчера: у нас `vars`/`presets`/`params`, у них `sections`/`selectable_rules`,
и DNS-серверы другой формы — поэтому донор падал на нашем файле.

ГАРАНТИЯ: после форматирования результат перечитывается и сравнивается с
исходником. Любое семантическое расхождение — выход с ошибкой БЕЗ записи:
форматтер не имеет права менять смысл шаблона.

    python3 scripts/format_wizard_template.py [path] [--check]

--check — только проверить, отличается ли файл от канонического вида
(exit 1 если да). Полезно в CI.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any

INDENT = "  "

# Порядок «шапки» var'а: §10.2 — эти ключи идут первой строкой.
VAR_HEADER_KEYS = ("name", "type", "wizard_ui", "title", "tooltip", "platforms",
                   "comment", "select", "required")


def compact(obj: Any) -> str:
    return json.dumps(obj, ensure_ascii=False, separators=(", ", ": "))


def pad(level: int) -> str:
    return INDENT * level


def is_at(v: Any) -> bool:
    """Значение-выражение: ссылка на var либо control-construct."""
    return isinstance(v, str) and v.startswith("@")


def has_expr(obj: Any) -> bool:
    """Есть ли где-то внутри `@`-ссылка или `#if` — такие узлы разворачиваем."""
    if isinstance(obj, str):
        return obj.startswith("@")
    if isinstance(obj, dict):
        return any(k.startswith("#") for k in obj) or any(has_expr(v) for v in obj.values())
    if isinstance(obj, list):
        return any(has_expr(x) for x in obj)
    return False


def strip_trailing_commas(text: str) -> str:
    prev = None
    while prev != text:
        prev = text
        text = re.sub(r",(\s*[\]\}])", r"\1", text)
    return text


def comma(trailing: bool) -> str:
    return "," if trailing else ""


# --- vars ------------------------------------------------------------------

def format_var(v: dict[str, Any], level: int, *, trailing_comma: bool) -> list[str]:
    """§10.2: шапка одной строкой, default_value/options/if — отдельными."""
    if v.get("separator"):
        return [pad(level) + compact(v) + comma(trailing_comma)]

    # Простой var без выражений и списков — целиком в одну строку.
    tail_keys = {"default_value", "default", "options", "if", "if_or", "default_node"}
    if not (set(v) & tail_keys) or (
        set(v) <= set(VAR_HEADER_KEYS) | {"default_value", "default"}
        and not has_expr(v.get("default_value", v.get("default")))
        and len(compact(v)) <= 110
    ):
        return [pad(level) + compact(v) + comma(trailing_comma)]

    header = ", ".join(f'"{k}": {json.dumps(v[k], ensure_ascii=False)}'
                       for k in VAR_HEADER_KEYS if k in v)
    lines = [pad(level) + "{" + header + ","]

    for key in ("default_value", "default", "default_node"):
        if key in v:
            lines.append(pad(level + 1) + f'"{key}": {compact(v[key])},')

    if "options" in v:
        opts = v["options"]
        # §10.3: литеральные options — одной строкой; объектные — по элементу.
        if opts and all(isinstance(o, str) for o in opts) and len(compact(opts)) <= 100:
            lines.append(pad(level + 1) + f'"options": {compact(opts)},')
        else:
            lines.append(pad(level + 1) + '"options": [')
            for i, o in enumerate(opts):
                lines.append(pad(level + 2) + compact(o) + comma(i < len(opts) - 1))
            lines.append(pad(level + 1) + "],")

    # §10.2: outer if/if_or — отдельной строкой В КОНЦЕ объекта.
    for key in ("if", "if_or"):
        if key in v:
            lines.append(pad(level + 1) + f'"{key}": {compact(v[key])},')

    # Всё, что схема допускает, но §10 явно не упоминает — не теряем.
    known = set(VAR_HEADER_KEYS) | tail_keys | {"separator"}
    for k in v:
        if k not in known:
            lines.append(pad(level + 1) + f'"{k}": {compact(v[k])},')

    lines[-1] = lines[-1].rstrip(",")
    lines.append(pad(level) + "}" + comma(trailing_comma))
    return lines


def format_var_array(vars_: list[dict[str, Any]], level: int, key: str = "vars") -> list[str]:
    lines = [pad(level) + f'"{key}": [']
    for i, v in enumerate(vars_):
        lines.extend(format_var(v, level + 1, trailing_comma=i < len(vars_) - 1))
    lines.append(pad(level) + "],")
    return lines


# --- payload (config / params[].value / presets) ---------------------------

def format_value(obj: Any, level: int, *, trailing_comma: bool, key: str | None = None) -> list[str]:
    """§10.3–10.4: разворачиваем то, что содержит выражения; остальное компактно."""
    prefix = f'"{key}": ' if key else ""

    if not has_expr(obj) and len(compact(obj)) <= 100:
        return [pad(level) + prefix + compact(obj) + comma(trailing_comma)]

    if isinstance(obj, dict):
        # §10.4: #if со скалярным value — одной строкой.
        if set(obj) == {"#if"} and not isinstance(obj["#if"].get("value"), (dict, list)):
            return [pad(level) + prefix + compact(obj) + comma(trailing_comma)]

        lines = [pad(level) + prefix + "{"]
        items = list(obj.items())
        for i, (k, v) in enumerate(items):
            lines.extend(format_value(v, level + 1, trailing_comma=i < len(items) - 1, key=k))
        lines.append(pad(level) + "}" + comma(trailing_comma))
        return lines

    if isinstance(obj, list):
        if not has_expr(obj) and len(compact(obj)) <= 100:
            return [pad(level) + prefix + compact(obj) + comma(trailing_comma)]
        lines = [pad(level) + prefix + "["]
        for i, item in enumerate(obj):
            lines.extend(format_value(item, level + 1, trailing_comma=i < len(obj) - 1))
        lines.append(pad(level) + "]" + comma(trailing_comma))
        return lines

    return [pad(level) + prefix + json.dumps(obj, ensure_ascii=False) + comma(trailing_comma)]


# --- presets ---------------------------------------------------------------

def format_preset(p: dict[str, Any], level: int, *, trailing_comma: bool) -> list[str]:
    """§10.5: metadata компактно, vars/rule_set/dns_servers — по своим правилам."""
    head_keys = ("id", "label", "description", "default_enabled", "platforms")
    header = ", ".join(f'"{k}": {json.dumps(p[k], ensure_ascii=False)}'
                       for k in head_keys if k in p)
    lines = [pad(level) + "{" + header + ","]

    if "vars" in p:
        lines.extend(format_var_array(p["vars"], level + 1))

    rest = [k for k in p if k not in head_keys and k != "vars"]
    for i, k in enumerate(rest):
        lines.extend(format_value(p[k], level + 1, trailing_comma=i < len(rest) - 1, key=k))

    lines[-1] = lines[-1].rstrip(",")
    lines.append(pad(level) + "}" + comma(trailing_comma))
    return lines


# --- top level -------------------------------------------------------------

def format_template(data: dict[str, Any]) -> str:
    out: list[str] = ["{"]
    keys = list(data)

    for idx, section in enumerate(keys):
        last = idx == len(keys) - 1
        value = data[section]

        if section == "vars":
            lines = format_var_array(value, 1)
            if last:
                lines[-1] = lines[-1].rstrip(",")
            out.extend(lines)
        elif section == "presets":
            out.append(pad(1) + '"presets": [')
            for i, p in enumerate(value):
                out.extend(format_preset(p, 2, trailing_comma=i < len(value) - 1))
            out.append(pad(1) + "]" + comma(not last))
        else:
            out.extend(format_value(value, 1, trailing_comma=not last, key=section))

    out.append("}")
    return strip_trailing_commas("\n".join(out))


def main() -> None:
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    check_only = "--check" in sys.argv

    path = Path(__file__).resolve().parents[1] / "bin" / "wizard_template.json"
    if args:
        path = Path(args[0])

    raw = path.read_text(encoding="utf-8")
    original = json.loads(raw)
    formatted = format_template(original) + "\n"

    # Единственная защита, которая здесь важна: форматтер не меняет смысл.
    # Сравниваем распарсенные деревья, а не текст.
    if json.loads(formatted) != original:
        print("ERROR: semantic mismatch after format — файл НЕ записан", file=sys.stderr)
        sys.exit(1)

    if check_only:
        if formatted != raw:
            print(f"{path}: не соответствует §10 (прогоните форматтер)", file=sys.stderr)
            sys.exit(1)
        print(f"{path}: OK")
        return

    if formatted == raw:
        print(f"{path}: уже отформатирован")
        return

    path.write_text(formatted, encoding="utf-8")
    print(f"Formatted {path}")


if __name__ == "__main__":
    main()
