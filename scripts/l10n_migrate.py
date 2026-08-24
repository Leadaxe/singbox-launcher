#!/usr/bin/env python3
"""SPEC 111: миграция локализации на естественные ключи.

Одноразовый инструмент инкрементального перехода (PLAN §«Порядок»).

  --catalog   добавить в bin/locale/ru.json записи с естественными ключами
              для всех легаси-ключей en.json (легаси-записи остаются рядом
              до финального шага); коллизии (один английский текст → разные
              русские переводы) разводятся по special-формам, соответствие
              старый ключ → индекс формы пишется в scripts/l10n_collisions.json
  --code      переписать литеральные вызовы locale.T/Tf с легаси-ключей на
              естественные (по en.json + l10n_collisions.json); длинные и
              многострочные значения инлайнятся и попадают в отчёт для
              последующего выноса в const
  --strip     финальный шаг: удалить легаси-записи из ru.json

Запуск из корня репозитория: python3 scripts/l10n_migrate.py --catalog
"""

import argparse
import collections
import json
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
EN = ROOT / "internal/locale/en.json"
RU = ROOT / "bin/locale/ru.json"
COLLISIONS = ROOT / "scripts/l10n_collisions.json"

DISPLAY_NAME = "_display_name"
LONG_LIMIT = 120  # длиннее — кандидат на const (вычитка субагентом)

CALL_RE = re.compile(r'locale\.(T|Tf)\("([a-z0-9_.]+)"')


def load(path):
    return json.loads(path.read_text(encoding="utf-8"))


def legacy_items(cat):
    """Легаси-записи: плоские строки со «точечным» ключом."""
    for k, v in cat.items():
        if k == DISPLAY_NAME or not isinstance(v, str):
            continue
        yield k, v


def build_collisions(en, ru):
    """Группировка легаси-ключей по английскому значению.

    Возвращает (natural: en_text -> {form -> ru_text},
                key_form: old_key -> form_index).
    Форма 0 — самый частый перевод (при равенстве — лексикографически
    первый), остальные получают 1, 2, … в порядке убывания частоты.
    """
    groups = collections.defaultdict(list)  # en_text -> [(old_key, ru_text)]
    for k, v in legacy_items(en):
        groups[v].append((k, ru.get(k, "")))

    natural, key_form = {}, {}
    for en_text, pairs in groups.items():
        freq = collections.Counter(ru_text for _, ru_text in pairs)
        variants = sorted(freq, key=lambda t: (-freq[t], t))
        forms = {i: t for i, t in enumerate(variants)}
        natural[en_text] = forms
        for old_key, ru_text in pairs:
            key_form[old_key] = variants.index(ru_text)
    return natural, key_form


def cmd_catalog():
    en, ru = load(EN), load(RU)
    natural, key_form = build_collisions(en, ru)

    collided = {t: f for t, f in natural.items() if len(f) > 1}
    print(f"легаси-ключей: {sum(1 for _ in legacy_items(en))}, "
          f"естественных: {len(natural)}, коллизий: {len(collided)}")
    for t, forms in sorted(collided.items()):
        print(f"  {t!r}: " + "; ".join(f"[{i}] {v}" for i, v in forms.items()))

    # Записи с естественными ключами — после легаси-блока. Существующие
    # (например, из демо-миграции) не перезаписываем.
    added = 0
    for en_text, forms in natural.items():
        if en_text in ru:
            continue
        entry = {"value": forms[0]}
        if len(forms) > 1:
            entry["special"] = {str(i): {"value": v}
                               for i, v in forms.items() if i > 0}
        ru[en_text] = entry
        added += 1

    RU.write_text(json.dumps(ru, ensure_ascii=False, indent=2) + "\n",
                  encoding="utf-8")
    COLLISIONS.write_text(json.dumps(
        {k: f for k, f in sorted(key_form.items()) if f > 0},
        ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"добавлено записей в ru.json: {added}; "
          f"ключей с формой >0: {sum(1 for f in key_form.values() if f)} "
          f"→ {COLLISIONS.name}")


def go_escape(s):
    return (s.replace("\\", "\\\\").replace('"', '\\"')
             .replace("\n", "\\n").replace("\t", "\\t"))


def cmd_code():
    en = load(EN)
    key_form = {k: v for k, v in load(COLLISIONS).items()} if COLLISIONS.exists() else {}
    en_flat = dict(legacy_items(en))

    long_sites = []   # (file, old_key) — кандидаты на const
    misses = []       # ключ в коде, но не в en.json
    changed_files, replaced = set(), 0

    def repl(m, path):
        nonlocal replaced
        fn, key = m.group(1), m.group(2)
        text = en_flat.get(key)
        if text is None:
            misses.append((str(path), key))
            return m.group(0)
        form = key_form.get(key, 0)
        if len(text) > LONG_LIMIT or "\n" in text:
            long_sites.append((str(path), key))
        lit = f'"{go_escape(text)}"'
        replaced += 1
        changed_files.add(path)
        if form:
            return f"locale.{fn}N({form}, {lit}"
        return f"locale.{fn}({lit}"

    for path in sorted(ROOT.rglob("*.go")):
        rel = path.relative_to(ROOT)
        if rel.parts[0] in ("dist", "temp", "tools") or path.name.endswith("_test.go"):
            continue
        src = path.read_text(encoding="utf-8")
        out = CALL_RE.sub(lambda m: repl(m, rel), src)
        if out != src:
            path.write_text(out, encoding="utf-8")

    print(f"заменено вызовов: {replaced} в {len(changed_files)} файлах")
    if misses:
        print(f"ключи не из en.json ({len(misses)}):")
        for f, k in misses:
            print(f"  {f}: {k}")
    if long_sites:
        report = ROOT / "scripts/l10n_long_sites.txt"
        report.write_text("\n".join(f"{f}\t{k}" for f, k in sorted(set(long_sites))) + "\n",
                          encoding="utf-8")
        print(f"длинных/многострочных сайтов: {len(set(long_sites))} → {report.name} "
              f"(вынос в const — отдельная вычитка)")
    if changed_files:
        subprocess.run(["gofmt", "-w"] + [str(ROOT / f) for f in sorted(changed_files)],
                       check=True)


def cmd_strip():
    en, ru = load(EN), load(RU)
    legacy = {k for k, _ in legacy_items(en)}
    before = len(ru)
    ru = {k: v for k, v in ru.items() if k not in legacy}
    RU.write_text(json.dumps(ru, ensure_ascii=False, indent=2) + "\n",
                  encoding="utf-8")
    print(f"снято легаси-записей: {before - len(ru)}; осталось: {len(ru)}")


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument("--catalog", action="store_true")
    g.add_argument("--code", action="store_true")
    g.add_argument("--strip", action="store_true")
    args = ap.parse_args()
    if args.catalog:
        cmd_catalog()
    elif args.code:
        cmd_code()
    else:
        cmd_strip()


if __name__ == "__main__":
    sys.exit(main())
