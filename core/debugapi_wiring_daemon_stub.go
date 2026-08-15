//go:build !darwin

package core

import "singbox-launcher/core/debugapi"

// debugAPIDaemonFacade — на платформах без демонного движка группа /daemon/*
// не существует: nil означает «не регистрировать» (SPEC 100 §3.6).
func (ac *AppController) debugAPIDaemonFacade() debugapi.DaemonFacade {
	return nil
}
