package store

import (
	"context"
	"testing"
	"time"
)

func TestAgentHealthState_PorDefectoNoCiego(t *testing.T) {
	s := openTestStore(t)
	blind, err := s.AgentHealthState(context.Background())
	if err != nil {
		t.Fatalf("AgentHealthState: %v", err)
	}
	if blind {
		t.Fatal("blind = true, want false (recién migrado)")
	}
}

// El estado debe sobrevivir una reapertura del archivo (simula un
// reinicio del proceso): es justo lo que AGENT_BLIND necesita para no
// repetirse tras un reinicio (ver agent/alerts.EvaluateHealth).
func TestAgentHealthState_SobreviveReabrirElArchivo(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/agent.db"
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	s1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s1.SetAgentHealthState(ctx, true, "net1", at); err != nil {
		t.Fatalf("SetAgentHealthState: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reabrir: %v", err)
	}
	defer s2.Close()
	blind, err := s2.AgentHealthState(ctx)
	if err != nil {
		t.Fatalf("AgentHealthState tras reabrir: %v", err)
	}
	if !blind {
		t.Fatal("blind = false, want true: el estado debe sobrevivir un reinicio del proceso")
	}
}

func TestSetAgentHealthState_Recupera(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if err := s.SetAgentHealthState(ctx, true, "net1", at); err != nil {
		t.Fatalf("SetAgentHealthState(blind): %v", err)
	}
	if blind, err := s.AgentHealthState(ctx); err != nil || !blind {
		t.Fatalf("blind=%v err=%v, want true,nil", blind, err)
	}

	if err := s.SetAgentHealthState(ctx, false, "net1", at.Add(time.Hour)); err != nil {
		t.Fatalf("SetAgentHealthState(recuperado): %v", err)
	}
	if blind, err := s.AgentHealthState(ctx); err != nil || blind {
		t.Fatalf("blind=%v err=%v, want false,nil", blind, err)
	}
}
