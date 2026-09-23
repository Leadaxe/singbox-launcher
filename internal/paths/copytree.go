package paths

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// migratingSuffix — суффикс временного каталога копирования (SPEC 135 §3.4, §4.2).
const migratingSuffix = ".migrating"

// maxSkippedExamples — сколько пропущенных путей попадает в отчёт.
const maxSkippedExamples = 5

// CopyReport — итог копирования.
type CopyReport struct {
	Files, Dirs, Skipped int
	Bytes                int64
	SkippedExamples      []string // до 5 путей (относительно src) с причиной через ": "
}

func (r *CopyReport) skip(rel string, reason error) {
	r.Skipped++
	if len(r.SkippedExamples) < maxSkippedExamples {
		r.SkippedExamples = append(r.SkippedExamples, filepath.ToSlash(rel)+": "+reason.Error())
	}
}

// dstError — сбой на стороне назначения: копия не может быть полной,
// продвигать её нельзя.
type dstError struct{ err error }

func (e dstError) Error() string { return e.err.Error() }
func (e dstError) Unwrap() error { return e.err }

// CopyTree копирует дерево src в dst через временный каталог dst+".migrating"
// (общий копировщик миграции §3.4 и переключателя Portable §4.2):
//  1. стирает залежавшийся dst+".migrating" от прошлого обрыва;
//  2. обходит src: каталоги, обычные файлы и символьные ссылки (ссылка
//     воссоздаётся с тем же target, не разворачивается). Нечитаемое
//     (lstat/open/read/readlink) и не-файлы (сокеты, FIFO, устройства)
//     пропускаются со счётом в Skipped — копирование не прерывается;
//  3. продвигает результат: dst нет — rename tmp → dst; dst есть — каждый
//     элемент верхнего уровня tmp заменяет одноимённый в dst (источник
//     побеждает: это данные пользователя), затем tmp удаляется.
//
// Права переносятся с исходных с одной поправкой: владелец всегда получает
// rwx на каталоги и rw на файлы, иначе копию нельзя было бы ни дописывать,
// ни потом заменить этим же копировщиком. 0700 у daemon и 0600 у ключей
// сохраняются как есть. На Windows биты режима условны, особой ветки нет.
//
// Ошибка возвращается, только когда копирование невозможно как таковое:
// src не каталог или не читается, не создаётся tmp, сбой записи в tmp,
// не удалось продвижение. Кроме сбоя посреди слияния шага 3, tmp при
// ошибке стирается, а dst остаётся нетронутым.
func CopyTree(src, dst string) (CopyReport, error) {
	var rep CopyReport

	srcInfo, err := os.Stat(src)
	if err != nil {
		return rep, fmt.Errorf("copy %s: %w", src, err)
	}
	if !srcInfo.IsDir() {
		return rep, fmt.Errorf("copy %s: not a directory", src)
	}

	tmp := dst + migratingSuffix
	if err := os.RemoveAll(tmp); err != nil {
		return rep, fmt.Errorf("remove stale %s: %w", tmp, err)
	}
	if err := os.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		return rep, fmt.Errorf("create parent of %s: %w", tmp, err)
	}

	type dirPerm struct {
		path string
		perm os.FileMode
	}
	var dirs []dirPerm

	walkErr := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		rel, relErr := filepath.Rel(src, p)
		if relErr != nil {
			return dstError{relErr}
		}
		target := filepath.Join(tmp, rel)

		if err != nil {
			if rel == "." {
				return err // корень не читается — копировать нечего
			}
			rep.skip(rel, err)
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		mode := info.Mode()
		switch {
		case mode.IsDir():
			if err := os.Mkdir(target, 0o700); err != nil {
				return dstError{err}
			}
			dirs = append(dirs, dirPerm{target, mode.Perm() | 0o700})
			if rel != "." {
				rep.Dirs++
			}
			return nil

		case mode&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				rep.skip(rel, err)
				return nil
			}
			// Ссылку не создать (Windows без привилегии) — пропуск, не отказ.
			if err := os.Symlink(link, target); err != nil {
				rep.skip(rel, err)
				return nil
			}
			rep.Files++
			return nil

		case mode.IsRegular():
			n, err := copyFile(p, target, mode.Perm()|0o600)
			if err != nil {
				var de dstError
				if errors.As(err, &de) {
					return de
				}
				rep.skip(rel, err)
				return nil
			}
			rep.Files++
			rep.Bytes += n
			return nil

		default:
			rep.skip(rel, fmt.Errorf("unsupported file type %s", mode.Type()))
			return nil
		}
	})
	if walkErr != nil {
		_ = os.RemoveAll(tmp)
		return rep, fmt.Errorf("copy %s -> %s: %w", src, tmp, walkErr)
	}

	// Права каталогов — после наполнения, глубокие первыми (Walk идёт
	// в прямом порядке, значит обратный обход ставит детей раньше родителей).
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i].path, dirs[i].perm); err != nil {
			_ = os.RemoveAll(tmp)
			return rep, fmt.Errorf("chmod %s: %w", dirs[i].path, err)
		}
	}

	if err := promote(tmp, dst, dirs[0].perm); err != nil {
		return rep, err
	}
	return rep, nil
}

// copyFile копирует обычный файл. Ошибка чтения источника возвращается как
// есть (пропуск файла), ошибка записи — dstError (отказ всего копирования).
func copyFile(src, dst string, perm os.FileMode) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, dstError{err}
	}
	fail := func(err error) (int64, error) {
		_ = out.Close()
		_ = os.Remove(dst)
		return 0, err
	}

	var total int64
	buf := make([]byte, 64*1024)
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return fail(dstError{werr})
			}
			total += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fail(rerr)
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return 0, dstError{err}
	}
	// Chmod после записи: режим открытия урезается umask'ом.
	if err := os.Chmod(dst, perm); err != nil {
		return 0, dstError{err}
	}
	return total, nil
}

// promote — шаг 3: tmp становится dst.
func promote(tmp, dst string, rootPerm os.FileMode) error {
	info, err := os.Lstat(dst)
	switch {
	case os.IsNotExist(err):
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("rename %s -> %s: %w", tmp, dst, err)
		}
		return nil
	case err != nil:
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("stat %s: %w", dst, err)
	case !info.IsDir():
		// На месте каталога файл или ссылка — заменяем целиком.
		if err := os.RemoveAll(dst); err != nil {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("remove %s: %w", dst, err)
		}
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.RemoveAll(tmp)
			return fmt.Errorf("rename %s -> %s: %w", tmp, dst, err)
		}
		return nil
	}

	f, err := os.Open(tmp)
	if err != nil {
		return fmt.Errorf("open %s: %w", tmp, err)
	}
	names, err := f.Readdirnames(-1)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("read %s: %w", tmp, err)
	}
	for _, name := range names {
		from := filepath.Join(tmp, name)
		to := filepath.Join(dst, name)
		if err := os.RemoveAll(to); err != nil {
			return fmt.Errorf("remove %s: %w", to, err)
		}
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", from, to, err)
		}
	}
	if err := os.Remove(tmp); err != nil {
		return fmt.Errorf("remove %s: %w", tmp, err)
	}
	if err := os.Chmod(dst, rootPerm); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}
	return nil
}
