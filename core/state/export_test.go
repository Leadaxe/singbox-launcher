package state

// export_test.go — внутренние ручки для внешних тестов пакета (state_test).

// PurgeLegacyForTest — прямой доступ сценария §4.B.8 к коду сноса (шаг 8
// миграции): в проде он гейтится migrationPurgesLegacy=false до W5, а код
// обязан быть проверен уже в W2.
func PurgeLegacyForTest(s *State, lc LoadContext) {
	purgeLegacyAfterMigration(s, lc)
}

// DeriveLoadContextForTest — проверка вывода путей контекста из пути файла.
func DeriveLoadContextForTest(path string) LoadContext {
	return deriveLoadContext(path)
}
