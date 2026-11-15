package store

import (
	"context"
	"testing"
	"time"

	"github.com/baxner/vigia/agent/history"
	"github.com/baxner/vigia/agent/internal/ptrs"
	"github.com/baxner/vigia/core"
)

// Puerto a nivel de integración de RoomHistoryRepositoryTest.kt: la
// reconciliación y la gracia ya están probadas aparte sin base
// (agent/history); esto verifica que RecordScan —transacción, serialización
// de core.Device a JSON y lectura de vuelta— funciona de punta a punta
// contra SQLite real (en un archivo temporal).

var devicesT0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

var testNet = NetworkInfo{
	Key:     history.NewNetworkKey("192.168.0.1", "192.168.0.0/24"),
	Subnet:  "192.168.0.0/24",
	Gateway: "192.168.0.1",
}

func scanDev(ip, mac string) core.Device { return core.Device{IP: ip, MAC: mac} }

func TestRecordScan_SinConocidosCreaEquipoYDisparaFirstSeen(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	escaneado := scanDev("192.168.0.10", "aa:bb:cc:dd:ee:01")
	events, err := s.RecordScan(ctx, testNet, []core.Device{escaneado}, devicesT0)
	if err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	if len(events) != 1 || events[0].Event.Type != history.FirstSeen {
		t.Fatalf("eventos = %+v, want [FIRST_SEEN]", events)
	}

	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil {
		t.Fatalf("DevicesByNetwork: %v", err)
	}
	if len(known) != 1 {
		t.Fatalf("known = %+v, want 1 equipo", known)
	}
	d := known[0]
	if d.LastIP != "192.168.0.10" {
		t.Fatalf("lastIp = %q, want 192.168.0.10", d.LastIP)
	}
	if d.MAC == nil || *d.MAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("mac = %v, want aa:bb:cc:dd:ee:01", d.MAC)
	}
	if !d.Online {
		t.Fatal("esperaba online=true")
	}
}

func TestRecordScan_SegundoEscaneoIdenticoNoGeneraEventos(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	escaneado := scanDev("192.168.0.10", "aa:bb:cc:dd:ee:01")
	if _, err := s.RecordScan(ctx, testNet, []core.Device{escaneado}, devicesT0); err != nil {
		t.Fatalf("RecordScan 1: %v", err)
	}

	events, err := s.RecordScan(ctx, testNet, []core.Device{escaneado}, devicesT0.Add(30*time.Second))
	if err != nil {
		t.Fatalf("RecordScan 2: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("eventos = %+v, want ninguno", events)
	}
	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil {
		t.Fatalf("DevicesByNetwork: %v", err)
	}
	if len(known) != 1 {
		t.Fatalf("known = %+v, want 1 equipo (no debe duplicarse)", known)
	}
}

func TestRecordScan_CambiaDeIPYSeReconocePorMACConIPChanged(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mac := "aa:bb:cc:dd:ee:01"

	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.10", mac)}, devicesT0); err != nil {
		t.Fatalf("RecordScan 1: %v", err)
	}

	events, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.99", mac)}, devicesT0.Add(30*time.Second))
	if err != nil {
		t.Fatalf("RecordScan 2: %v", err)
	}
	if len(events) != 1 || events[0].Event.Type != history.IPChanged {
		t.Fatalf("eventos = %+v, want [IP_CHANGED]", events)
	}
	if events[0].Event.Detail == nil || *events[0].Event.Detail != "192.168.0.10 → 192.168.0.99" {
		t.Fatalf("detail = %v, want %q", events[0].Event.Detail, "192.168.0.10 → 192.168.0.99")
	}

	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil {
		t.Fatalf("DevicesByNetwork: %v", err)
	}
	if len(known) != 1 || known[0].LastIP != "192.168.0.99" {
		t.Fatalf("known = %+v, want lastIp=192.168.0.99", known)
	}
	id := known[0].ID

	// observeEvents (Events aquí): toda la cronología, más reciente primero:
	// IP_CHANGED (escaneo 2) y, antes, el FIRST_SEEN del primer escaneo.
	hist, err := s.Events(ctx, id, 0)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(hist) != 2 || hist[0].Type != history.IPChanged || hist[1].Type != history.FirstSeen {
		t.Fatalf("historial = %+v, want [IP_CHANGED, FIRST_SEEN]", hist)
	}
}

func TestRecordScan_GraciaPersisteEntreEscaneos_FaltaDosVecesYVuelve(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mac := "aa:bb:cc:dd:ee:01"

	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.10", mac)}, devicesT0); err != nil {
		t.Fatalf("RecordScan inicial: %v", err)
	}

	// Falta 1: sin el equipo en el escaneo de su red. Sin evento todavía.
	falta1, err := s.RecordScan(ctx, testNet, nil, devicesT0.Add(60*time.Second))
	if err != nil {
		t.Fatalf("RecordScan falta1: %v", err)
	}
	if len(falta1) != 0 {
		t.Fatalf("falta1 eventos = %+v, want ninguno", falta1)
	}
	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil || len(known) != 1 {
		t.Fatalf("DevicesByNetwork: known=%+v err=%v", known, err)
	}
	if known[0].Online {
		t.Fatal("esperaba online=false tras la primera falta")
	}
	if known[0].OnlineSince == nil || !known[0].OnlineSince.Equal(devicesT0) {
		t.Fatalf("onlineSince = %v, want %v (se conserva dentro de la gracia)", known[0].OnlineSince, devicesT0)
	}

	// Falta 2: ahora sí se compromete el WENT_OFFLINE, fechado con el
	// último lastSeen real (t0), no con t0+120s.
	falta2, err := s.RecordScan(ctx, testNet, nil, devicesT0.Add(120*time.Second))
	if err != nil {
		t.Fatalf("RecordScan falta2: %v", err)
	}
	if len(falta2) != 1 || falta2[0].Event.Type != history.WentOffline {
		t.Fatalf("falta2 eventos = %+v, want [WENT_OFFLINE]", falta2)
	}
	if !falta2[0].Event.At.Equal(devicesT0) {
		t.Fatalf("at = %v, want %v", falta2[0].Event.At, devicesT0)
	}
	known, err = s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil || len(known) != 1 {
		t.Fatalf("DevicesByNetwork: known=%+v err=%v", known, err)
	}
	if known[0].OnlineSince != nil {
		t.Fatalf("onlineSince = %v, want nil", known[0].OnlineSince)
	}

	// Vuelve: CAME_ONLINE con onlineSince = ahora.
	ahora := devicesT0.Add(600 * time.Second)
	vuelve, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.10", mac)}, ahora)
	if err != nil {
		t.Fatalf("RecordScan vuelve: %v", err)
	}
	if len(vuelve) != 1 || vuelve[0].Event.Type != history.CameOnline {
		t.Fatalf("vuelve eventos = %+v, want [CAME_ONLINE]", vuelve)
	}
	known, err = s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil || len(known) != 1 {
		t.Fatalf("DevicesByNetwork: known=%+v err=%v", known, err)
	}
	if !known[0].Online {
		t.Fatal("esperaba online=true")
	}
	if known[0].OnlineSince == nil || !known[0].OnlineSince.Equal(ahora) {
		t.Fatalf("onlineSince = %v, want %v", known[0].OnlineSince, ahora)
	}

	// La cronología completa quedó grabada en orden (más reciente primero).
	fullHist, err := s.Events(ctx, known[0].ID, 0)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	wantTypes := []history.DeviceEventType{history.CameOnline, history.WentOffline, history.FirstSeen}
	if len(fullHist) != len(wantTypes) {
		t.Fatalf("historial = %+v, want %d eventos", fullHist, len(wantTypes))
	}
	for i, want := range wantTypes {
		if fullHist[i].Type != want {
			t.Fatalf("historial[%d] = %v, want %v (completo: %+v)", i, fullHist[i].Type, want, fullHist)
		}
	}
}

func TestRecordScan_MasDeDiezMinutosComprometeWentOfflineAunqueSeaPrimeraFalta(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mac := "aa:bb:cc:dd:ee:01"

	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.10", mac)}, devicesT0); err != nil {
		t.Fatalf("RecordScan inicial: %v", err)
	}

	events, err := s.RecordScan(ctx, testNet, nil, devicesT0.Add(11*time.Minute))
	if err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	if len(events) != 1 || events[0].Event.Type != history.WentOffline {
		t.Fatalf("eventos = %+v, want [WENT_OFFLINE]", events)
	}
}

func TestDevicesByNetwork_OrdenaPorIPNumericaNoComoTexto(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Como texto, "105" y "29" quedarían antes de "9". Numérico: 9, 29, 105.
	devs := []core.Device{
		scanDev("192.168.0.105", "aa:aa:aa:aa:aa:01"),
		scanDev("192.168.0.9", "aa:aa:aa:aa:aa:02"),
		scanDev("192.168.0.29", "aa:aa:aa:aa:aa:03"),
	}
	if _, err := s.RecordScan(ctx, testNet, devs, devicesT0); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}

	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil {
		t.Fatalf("DevicesByNetwork: %v", err)
	}
	var ips []string
	for _, d := range known {
		ips = append(ips, d.LastIP)
	}
	want := []string{"192.168.0.9", "192.168.0.29", "192.168.0.105"}
	if len(ips) != len(want) {
		t.Fatalf("ips = %v, want %v", ips, want)
	}
	for i := range want {
		if ips[i] != want[i] {
			t.Fatalf("ips = %v, want %v", ips, want)
		}
	}
}

func TestUpdateCustomization_PersisteYSeLeeDeVuelta(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.10", "aa:bb:cc:dd:ee:01")}, devicesT0); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil || len(known) != 1 {
		t.Fatalf("DevicesByNetwork: known=%+v err=%v", known, err)
	}
	id := known[0].ID

	updated, err := s.UpdateCustomization(ctx, id, CustomizationPatch{
		Name: ptrs.To("Laptop de prueba"), SetName: true,
		Important: true, SetImportant: true,
	})
	if err != nil {
		t.Fatalf("UpdateCustomization: %v", err)
	}
	if updated.Custom.Name == nil || *updated.Custom.Name != "Laptop de prueba" {
		t.Fatalf("name = %v, want \"Laptop de prueba\"", updated.Custom.Name)
	}
	if !updated.Custom.Important {
		t.Fatal("important = false, want true")
	}

	reread, err := s.DeviceByID(ctx, id)
	if err != nil {
		t.Fatalf("DeviceByID: %v", err)
	}
	if reread.Custom.Name == nil || *reread.Custom.Name != "Laptop de prueba" {
		t.Fatalf("tras releer: name = %v, want \"Laptop de prueba\"", reread.Custom.Name)
	}

	// Campo ausente (SetLocation=false): no toca lo que ya había.
	again, err := s.UpdateCustomization(ctx, id, CustomizationPatch{Location: ptrs.To("Sala"), SetLocation: true})
	if err != nil {
		t.Fatalf("UpdateCustomization (2): %v", err)
	}
	if again.Custom.Name == nil || *again.Custom.Name != "Laptop de prueba" {
		t.Fatalf("un PATCH de otro campo no debe borrar name, got %v", again.Custom.Name)
	}
	if again.Custom.Location == nil || *again.Custom.Location != "Sala" {
		t.Fatalf("location = %v, want Sala", again.Custom.Location)
	}

	// null explícito (SetName=true, Name=nil): borra.
	cleared, err := s.UpdateCustomization(ctx, id, CustomizationPatch{Name: nil, SetName: true})
	if err != nil {
		t.Fatalf("UpdateCustomization (3): %v", err)
	}
	if cleared.Custom.Name != nil {
		t.Fatalf("name = %v, want nil tras borrar", cleared.Custom.Name)
	}
}

func TestForget_BorraElEquipoYSusEventos(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if _, err := s.RecordScan(ctx, testNet, []core.Device{scanDev("192.168.0.10", "aa:bb:cc:dd:ee:01")}, devicesT0); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}
	known, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil || len(known) != 1 {
		t.Fatalf("DevicesByNetwork: known=%+v err=%v", known, err)
	}
	id := known[0].ID

	if err := s.Forget(ctx, id); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	after, err := s.DevicesByNetwork(ctx, testNet.Key)
	if err != nil {
		t.Fatalf("DevicesByNetwork: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("after = %+v, want vacío", after)
	}
	if _, err := s.DeviceByID(ctx, id); err != ErrNotFound {
		t.Fatalf("DeviceByID tras Forget: err=%v, want ErrNotFound", err)
	}
	evs, err := s.Events(ctx, id, 0)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("events = %+v, want vacío (cascada)", evs)
	}
}

func TestForget_IDInexistenteDevuelveErrNotFound(t *testing.T) {
	s := openTestStore(t)
	if err := s.Forget(context.Background(), 999); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRecordScan_ActualizaRedYMarcaUltimoEscaneo(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	netWithGW := testNet
	netWithGW.GatewayMAC = "28:cf:e9:00:00:01"
	if _, err := s.RecordScan(ctx, netWithGW, nil, devicesT0); err != nil {
		t.Fatalf("RecordScan: %v", err)
	}

	nets, err := s.Networks(ctx)
	if err != nil {
		t.Fatalf("Networks: %v", err)
	}
	if len(nets) != 1 {
		t.Fatalf("nets = %+v, want 1 red", nets)
	}
	n := nets[0]
	if n.Key != string(testNet.Key) {
		t.Fatalf("key = %q, want %q", n.Key, testNet.Key)
	}
	if n.GatewayMAC == nil || *n.GatewayMAC != "28:cf:e9:00:00:01" {
		t.Fatalf("gatewayMAC = %v, want 28:cf:e9:00:00:01", n.GatewayMAC)
	}
	if n.LastScanAt == nil || !n.LastScanAt.Equal(devicesT0) {
		t.Fatalf("lastScanAt = %v, want %v", n.LastScanAt, devicesT0)
	}
}
