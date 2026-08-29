package state

import (
	"os"
	"path/filepath"
	"testing"
)

// legacyFixtureCopy — копия легаси-фикстуры во временном каталоге.
//
// SPEC 118 W5: Load легаси-состояния теперь МИГРИРУЕТ его и переписывает файл
// на месте (шаг 8: снос raw-кэша и перенос умолчаний идут только ПОСЛЕ
// успешной записи v7). Читать фикстуру по её месту в репозитории значит
// уничтожать её первым же прогоном — тест обязан работать с копией.
func legacyFixtureCopy(t *testing.T, src string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture %s: %v", src, err)
	}
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("copy fixture %s: %v", src, err)
	}
	return dst
}
