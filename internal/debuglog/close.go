package debuglog

import (
	"log"
)

// RunAndLog executes fn and logs label-prefixed error if it fails.
func RunAndLog(label string, fn func() error) {
	if err := fn(); err != nil {
		log.Printf("%s: %v", label, err)
	}
}

// ReleaseLogFiles отпускает файлы в LogDir, которые держит не FileService:
// crash.log (копия дескриптора у runtime после EnableCrashOutput) и
// native-stderr.log (RedirectNativeStderr). Зовётся очисткой перед удалением
// LogDir (SPEC 135 §4.3): на Windows открытый файл не удалить. После вызова
// трасса паники и нативный stderr больше не пишутся в файлы.
func ReleaseLogFiles() {
	releaseCrashOutput()
	releaseNativeStderr()
}
