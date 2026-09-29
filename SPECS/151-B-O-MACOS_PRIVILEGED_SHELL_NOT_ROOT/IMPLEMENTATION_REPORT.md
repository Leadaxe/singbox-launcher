# IMPLEMENTATION REPORT 151

## Сделано

- Шелл привилегированного старта — `/bin/bash -p` (был `/bin/sh`).
- Тело сверяет euid с аргументом `$8` (`0` в проде) и отказывает с обоими
  uid в тексте.
- Отказ открыть `classic.log` называет системную причину.
- `PrivilegedStartArgs` принимает `shellUID`; argv вырос на два элемента
  (`-p` и uid шелла).

## Проверки

- `go build ./...` (macOS 26.7) — успешно.
- `go test ./internal/platform/ -run TestPrivilegedStartCommand -count=1` —
  успешно.
- Текст причины открытия проверен руками на несуществующем каталоге:
  `refused: cannot open …: No such file or directory`.

- Причина и правка измерены на macOS 26.7 под AEWP (таблица SPEC §2):
  текущая цепочка даёт шеллу euid 501, цепочка с `/bin/bash -p` — euid 0.

## Не проверено

- Старт classic TUN лаунчером с правкой: измерялись только uid шелла, ядро
  через новую цепочку не запускалось.
- macOS 27.
