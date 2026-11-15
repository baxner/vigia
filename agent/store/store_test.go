package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen_SchemaVacioYReabrir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.db")
	ctx := context.Background()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Networks != 0 || st.Devices != 0 || st.OnlineDevices != 0 || st.LastScanAt != nil {
		t.Fatalf("stats = %+v, want todo en cero/nil", st)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reabrir una base ya migrada no debe fallar ni reintentar el esquema.
	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reabrir: %v", err)
	}
	defer s2.Close()
}

func TestClients_CrearListarRevocarYBuscarPorToken(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	token := "token-de-prueba-bien-largo"
	hash := HashToken(token)
	c, err := s.CreateClient(ctx, "Galaxy S24 Ultra", hash, now)
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("esperaba un id != 0")
	}

	found, err := s.FindActiveClientByToken(ctx, token)
	if err != nil {
		t.Fatalf("FindActiveClientByToken: %v", err)
	}
	if found == nil || found.ID != c.ID {
		t.Fatalf("found = %+v, want id=%d", found, c.ID)
	}

	if got, err := s.FindActiveClientByToken(ctx, "otro-token-cualquiera"); err != nil || got != nil {
		t.Fatalf("token equivocado: got=%+v err=%v, want nil,nil", got, err)
	}

	n, err := s.RevokeByName(ctx, "Galaxy S24 Ultra", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("RevokeByName: %v", err)
	}
	if n != 1 {
		t.Fatalf("revocados = %d, want 1", n)
	}

	if got, err := s.FindActiveClientByToken(ctx, token); err != nil || got != nil {
		t.Fatalf("un cliente revocado no debe autenticar: got=%+v err=%v", got, err)
	}

	all, err := s.Clients(ctx)
	if err != nil {
		t.Fatalf("Clients: %v", err)
	}
	if len(all) != 1 || all[0].Active() {
		t.Fatalf("Clients = %+v, want exactamente 1 revocado", all)
	}
}

func TestPairing_CicloCompleto(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	pc, err := s.CreatePairingCode(ctx, now)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	if len(pc.Code) != 6 {
		t.Fatalf("code = %q, want 6 dígitos", pc.Code)
	}
	for _, r := range pc.Code {
		if r < '0' || r > '9' {
			t.Fatalf("code = %q, tiene un carácter no numérico", pc.Code)
		}
	}
	if !pc.ExpiresAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("expiresAt = %v, want %v", pc.ExpiresAt, now.Add(5*time.Minute))
	}

	wrong := "000000"
	if wrong == pc.Code {
		wrong = "111111"
	}
	if res, err := s.TryConsumePairingCode(ctx, wrong, now); err != nil || res != PairingWrongCode {
		t.Fatalf("código equivocado: res=%v err=%v, want PairingWrongCode", res, err)
	}

	res, err := s.TryConsumePairingCode(ctx, pc.Code, now)
	if err != nil {
		t.Fatalf("TryConsumePairingCode: %v", err)
	}
	if res != PairingOK {
		t.Fatalf("res = %v, want PairingOK", res)
	}

	// De un solo uso: repetir el mismo código ya no sirve.
	res2, err := s.TryConsumePairingCode(ctx, pc.Code, now)
	if err != nil {
		t.Fatalf("segundo intento: %v", err)
	}
	if res2 != PairingNoActiveCode {
		t.Fatalf("res2 = %v, want PairingNoActiveCode", res2)
	}
}

func TestPairing_Vence(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	pc, err := s.CreatePairingCode(ctx, now)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}

	res, err := s.TryConsumePairingCode(ctx, pc.Code, now.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("TryConsumePairingCode: %v", err)
	}
	if res != PairingExpired {
		t.Fatalf("res = %v, want PairingExpired", res)
	}
}

func TestPairing_MaximoCincoIntentos(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	pc, err := s.CreatePairingCode(ctx, now)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	wrong := "000000"
	if wrong == pc.Code {
		wrong = "111111"
	}

	// Los primeros pairingMaxAttempts intentos (equivocados) se evalúan de
	// verdad, uno por uno.
	for i := 1; i <= pairingMaxAttempts; i++ {
		res, err := s.TryConsumePairingCode(ctx, wrong, now)
		if err != nil {
			t.Fatalf("intento %d: %v", i, err)
		}
		if res != PairingWrongCode {
			t.Fatalf("intento %d = %v, want PairingWrongCode", i, res)
		}
	}

	// Se agotó el cupo: ni el código correcto original sirve ya (hace
	// falta un código nuevo, no una adivinada de suerte).
	res, err := s.TryConsumePairingCode(ctx, pc.Code, now)
	if err != nil {
		t.Fatalf("intento tras agotar cupo: %v", err)
	}
	if res != PairingAttemptsExhausted {
		t.Fatalf("res = %v, want PairingAttemptsExhausted", res)
	}

	// Y ya no queda código pendiente en absoluto.
	res2, err := s.TryConsumePairingCode(ctx, pc.Code, now)
	if err != nil {
		t.Fatalf("intento tras borrar: %v", err)
	}
	if res2 != PairingNoActiveCode {
		t.Fatalf("res2 = %v, want PairingNoActiveCode", res2)
	}
}

func TestCreatePairingCode_InvalidaElAnterior(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if _, err := s.CreatePairingCode(ctx, now); err != nil {
		t.Fatalf("CreatePairingCode (1): %v", err)
	}
	segundo, err := s.CreatePairingCode(ctx, now.Add(time.Second))
	if err != nil {
		t.Fatalf("CreatePairingCode (2): %v", err)
	}

	// A lo sumo una fila pendiente: pedir un código nuevo descarta el anterior.
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pairing_codes").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("pairing_codes tiene %d filas, want 1", count)
	}
	var current string
	if err := s.db.QueryRowContext(ctx, "SELECT code FROM pairing_codes").Scan(&current); err != nil {
		t.Fatalf("leer código vigente: %v", err)
	}
	if current != segundo.Code {
		t.Fatalf("código vigente = %q, want el segundo (%q)", current, segundo.Code)
	}
}
