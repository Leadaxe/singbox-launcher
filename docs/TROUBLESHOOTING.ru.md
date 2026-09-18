# Решение проблем

Известные проблемы и где лежат их решения. Быстрые первые проверки — в
[таблице в README](../README.ru.md#troubleshooting); проблемы сборки на Linux — в
[BUILD_LINUX.ru.md](BUILD_LINUX.ru.md).

English version: [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

## Linux

### Пароль спрашивают три раза при старте VPN и один раз при остановке

Десктоп с `systemd-resolved`: sing-box настраивает DNS TUN-интерфейса через `resolvectl`,
и Polkit авторизует каждое из четырёх D-Bus-действий отдельно. `CAP_NET_ADMIN` у бинаря не
помогает — проверка идёт на стороне `systemd-resolved`.

Рецепт от пользователя (отдельная группа и узкое правило Polkit ровно на эти четыре
действия) — в [issue #126](https://github.com/Leadaxe/singbox-launcher/issues/126). Это системная настройка, её делает администратор;
лаунчер её не устанавливает, и на других дистрибутивах проект её не проверял.
