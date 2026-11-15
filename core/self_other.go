//go:build !darwin && !ios && !linux

package core

// selfExtra: en otros SO no hay metadatos locales adicionales que añadir.
func selfExtra() map[string]string { return nil }
