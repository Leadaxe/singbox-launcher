package state

// Секции узла: подстановка плейсхолдера (SPEC 121 §10.1).
//
// Тест data-критичный: подстановка решает, каким тегом узел адресован в
// конфиге.

import "testing"

// TestSubstituteSelf — обе формы плейсхолдера, ключи, чужой текст, порядок.
func TestSubstituteSelf(t *testing.T) {
	const finalTag = "DE-ts-node"

	t.Run("whole string and embedded forms", func(t *testing.T) {
		in := []byte(`{"outbound":"@self","tag":"@{self}-dns","name":"@{self} network"}`)
		got := string(SubstituteSelf(in, finalTag))
		want := `{"outbound":"DE-ts-node","tag":"DE-ts-node-dns","name":"DE-ts-node network"}`
		if got != want {
			t.Errorf("SubstituteSelf =\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("key order survives", func(t *testing.T) {
		// Порядок ключей значим: тела сравниваются с выводом эмиттера
		// байт-в-байт (CODEMAP §10 п. 21).
		in := []byte(`{"zz":1,"aa":"@self","mm":[{"nested":"@{self}!"}]}`)
		got := string(SubstituteSelf(in, finalTag))
		want := `{"zz":1,"aa":"DE-ts-node","mm":[{"nested":"DE-ts-node!"}]}`
		if got != want {
			t.Errorf("SubstituteSelf =\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("foreign text and keys are data", func(t *testing.T) {
		// Ключ `@self` — имя поля, а не ссылка; `@selfish` — не плейсхолдер
		// (форма `@self` подставляется только целой строкой); чужая `@var`
		// остаётся собой: словаря переменных у секции нет.
		in := []byte(`{"@self":"key","a":"@selfish","b":"@other","c":"mail@self.example"}`)
		got := string(SubstituteSelf(in, finalTag))
		if got != string(in) {
			t.Errorf("SubstituteSelf тронул то, что плейсхолдером не является:\n%s\nwant\n%s", got, in)
		}
	})

	t.Run("empty final tag leaves the placeholder alone", func(t *testing.T) {
		// Подстановка пустой строкой дала бы `"outbound": ""` — висячую
		// ссылку вместо честной.
		in := []byte(`{"outbound":"@self"}`)
		if got := string(SubstituteSelf(in, "")); got != string(in) {
			t.Errorf("SubstituteSelf с пустым тегом = %s, want %s", got, in)
		}
	})
}
