package build

import (
	"fmt"
	"strconv"
	"strings"
)

// Переменные кэша DNS (SPEC 147, LxBox §580, контракт 1.1.93).
const (
	VarDNSCacheCapacity = "dns_cache_capacity"
	VarDNSOptimistic    = "dns_optimistic"
	VarDNSStoreCache    = "dns_store_cache"

	// DNSCacheCapacityMin / DNSCacheCapacityMax — допустимые значения
	// dns.cache_capacity (число записей). Ниже 1024 ядро само поднимает
	// значение до 1024.
	DNSCacheCapacityMin = 1024
	DNSCacheCapacityMax = 65536
)

// ValidDNSCacheCapacity true, если строка — целое число в допустимых границах.
func ValidDNSCacheCapacity(raw string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	return err == nil && n >= DNSCacheCapacityMin && n <= DNSCacheCapacityMax
}

// sanitizeDNSCacheVars снимает сохранённое значение dns_cache_capacity вне
// границ (правка state.json руками, старая версия): в конфиг уходит дефолт
// шаблона, а не негодное число. Карту вызывающего не меняет.
func sanitizeDNSCacheVars(vars map[string]string, res *Result) map[string]string {
	raw, ok := vars[VarDNSCacheCapacity]
	if !ok || ValidDNSCacheCapacity(raw) {
		return vars
	}
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		if k != VarDNSCacheCapacity {
			out[k] = v
		}
	}
	if res != nil {
		res.Validation.Warnings = append(res.Validation.Warnings,
			fmt.Sprintf("%s=%q is outside %d..%d; template default used",
				VarDNSCacheCapacity, raw, DNSCacheCapacityMin, DNSCacheCapacityMax))
	}
	return out
}
