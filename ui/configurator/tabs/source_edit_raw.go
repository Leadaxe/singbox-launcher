package tabs

// rawBodyMaxDisplay — лимит на отображение body в textarea (4 KB).
//
// Fyne widget.MultiLineEntry рендерит каждый символ через text-layout
// без виртуализации (см. https://github.com/fyne-io/fyne/issues/2935):
// уже на 64 KB Xray JSON (вложенные объекты, длинные строки) на macOS
// заметно лагает. 4 KB — это «инспектор первых нескольких нод» без
// freeze'ов.
//
// Покрытие:
//   - base64-decoded подписки на 10-15 URI строк (~4 KB) — целиком
//   - Xray JSON pretty-printed: первые 1-2 ноды (выглядит как структура,
//     но юзер быстро увидит truncated-label куда идти за полным)
//
// Для больших bodies полный raw доступен через filesystem path
// `bin/subscriptions/<id>.raw` — показываем подсказку в truncated label.
const rawBodyMaxDisplay = 4 * 1024
