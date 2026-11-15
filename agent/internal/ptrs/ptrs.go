// Package ptrs tiene un solo ayudante: tomar la dirección de un valor que no
// la tiene (literales, retornos de función) sin declarar una variable
// intermedia en cada sitio. Se usa por todo agent/ para los punteros
// opcionales de KnownDevice/DeviceCustomization/DeviceEvent (nil = ausente,
// ver docs/agente-api.md).
package ptrs

// To devuelve un puntero a una copia de v.
func To[T any](v T) *T { return &v }

// Deref devuelve *p, o zero si p es nil.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
