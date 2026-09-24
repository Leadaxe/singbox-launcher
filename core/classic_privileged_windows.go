//go:build windows && !386

package core

// showPrivilegedCopyDialog — TODO(SPEC 141 этап 6, после SPEC 139): тексты
// «administrator», команда copy/install через runas, лог classic.log (§8).
// Сейчас не вызывается: privilegedCoreCopyGate зовут только под darwin.
func (ac *AppController) showPrivilegedCopyDialog(_ privilegedCopyCheck, _ string, _ bool, _ string) {
}
