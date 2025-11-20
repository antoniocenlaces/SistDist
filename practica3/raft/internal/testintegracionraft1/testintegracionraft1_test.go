package testintegracionraft1

import (
	"fmt"
	"os"
	"path/filepath"
	"raft/internal/comun/check"

	//"log"
	//"crypto/rand"

	"strconv"
	"testing"
	"time"

	"raft/internal/comun/rpctimeout"
	"raft/internal/despliegue"
	"raft/internal/raft"
)

const (
	//hosts
	MAQUINA1 = "192.168.3.13"
	MAQUINA2 = "192.168.3.14"
	MAQUINA3 = "192.168.3.15"

	//puertos
	PUERTOREPLICA1 = "29280"
	PUERTOREPLICA2 = "29280"
	PUERTOREPLICA3 = "29280"

	//nodos replicas
	REPLICA1 = MAQUINA1 + ":" + PUERTOREPLICA1
	REPLICA2 = MAQUINA2 + ":" + PUERTOREPLICA2
	REPLICA3 = MAQUINA3 + ":" + PUERTOREPLICA3

	// paquete main de ejecutables relativos a PATH previo
	EXECREPLICA = "cmd/srvraft/main.go"

	// comandos completo a ejecutar en máquinas remota con ssh. Ejemplo :
	// 				cd $HOME/raft; go run cmd/srvraft/main.go 127.0.0.1:29001

	// Ubicar, en esta constante, nombre de fichero de vuestra clave privada local
	// emparejada con la clave pública en authorized_keys de máquinas remotas

	PRIVKEYFILE = "id_ed25519"
)

// PATH de los ejecutables de modulo golang de servicio Raft
// var PATH string = filepath.Join(os.Getenv("HOME"), "tmp", "p3", "raft")
var PATH string = filepath.Join(os.Getenv("PWD"), "..", "..")

// var PATH string = "/misc/alumnos/sd/sd2526/a143045/SistDist/practica3/raft"

// go run cmd/srvraft/main.go 0 127.0.0.1:29001 127.0.0.1:29002 127.0.0.1:29003
var EXECREPLICACMD string = "cd " + PATH + "; go run " + EXECREPLICA

// TEST primer rango
func TestPrimerasPruebas(t *testing.T) { // (m *testing.M) {
	// <setup code>
	// Crear canal de resultados de ejecuciones ssh en maquinas remotas
	cfg := makeCfgDespliegue(t,
		3,
		[]string{REPLICA1, REPLICA2, REPLICA3},
		// sistema de test arranc con nodos parados
		// en el arranque se marca quién está activo
		[]bool{false, false, false})

	// tear down code
	// eliminar procesos en máquinas remotas
	defer cfg.stop()

	// Run test sequence

	// Test1 : No debería haber ningun primario, si SV no ha recibido aún latidos
	t.Run("T1:soloArranqueYparada",
		func(t *testing.T) {
			// para garantizar que aunque el test falle se paran los nodos
			// que queden activos
			defer cfg.stopDistributedProcesses()
			cfg.soloArranqueYparadaTest1(t)
		})

	// Test2 : No debería haber ningun primario, si SV no ha recibido aún latidos
	t.Run("T2:ElegirPrimerLider",
		func(t *testing.T) {
			defer cfg.stopDistributedProcesses()
			cfg.elegirPrimerLiderTest2(t)
		})

	// Test3: tenemos el primer primario correcto
	t.Run("T3:FalloAnteriorElegirNuevoLider",
		func(t *testing.T) {
			defer cfg.stopDistributedProcesses()
			cfg.falloAnteriorElegirNuevoLiderTest3(t)
		})

	// Test4: Tres operaciones comprometidas en configuración estable
	t.Run("T4:tresOperacionesComprometidasEstable",
		func(t *testing.T) {
			defer cfg.stopDistributedProcesses()
			cfg.tresOperacionesComprometidasEstable(t)
		})
}

// TEST primer rango
func TestAcuerdosConFallos(t *testing.T) { // (m *testing.M) {
	// <setup code>
	// Crear canal de resultados de ejecuciones ssh en maquinas remotas
	cfg := makeCfgDespliegue(t,
		3,
		[]string{REPLICA1, REPLICA2, REPLICA3},
		[]bool{false, false, false})

	// tear down code
	// eliminar procesos en máquinas remotas
	defer cfg.stop()

	// Test5: Se consigue acuerdo a pesar de desconexiones de seguidor
	t.Run("T5:AcuerdoAPesarDeDesconexionesDeSeguidor ",
		func(t *testing.T) {
			defer cfg.stopDistributedProcesses()
			cfg.AcuerdoApesarDeSeguidor(t)
		})

	t.Run("T5:SinAcuerdoPorFallos ",
		func(t *testing.T) {
			defer cfg.stopDistributedProcesses()
			cfg.SinAcuerdoPorFallos(t)
		})

	t.Run("T5:SometerConcurrentementeOperaciones ",
		func(t *testing.T) {
			defer cfg.stopDistributedProcesses()
			cfg.SometerConcurrentementeOperaciones(t)
		})

}

// ---------------------------------------------------------------------
//
// Canal de resultados de ejecución de comandos ssh remotos
type canalResultados chan string

func (cr canalResultados) stop() {
	close(cr)

	// Leer las salidas obtenidos de los comandos ssh ejecutados
	for s := range cr {
		fmt.Println(s)
	}
}

// ---------------------------------------------------------------------
// Operativa en configuracion de despliegue y pruebas asociadas
type configDespliegue struct {
	t           *testing.T
	conectados  []bool
	numReplicas int
	nodosRaft   []rpctimeout.HostPort
	cr          canalResultados
}

// Crear una configuracion de despliegue
func makeCfgDespliegue(t *testing.T, n int, nodosraft []string,
	conectados []bool) *configDespliegue {
	cfg := &configDespliegue{}
	cfg.t = t
	cfg.conectados = conectados
	cfg.numReplicas = n
	cfg.nodosRaft = rpctimeout.StringArrayToHostPortArray(nodosraft)
	cfg.cr = make(canalResultados, 2000)

	return cfg
}

func (cfg *configDespliegue) stop() {
	// la rutina que para los procesos controla los que están activos
	// usando cfg.conectados[]
	cfg.stopDistributedProcesses()

	time.Sleep(50 * time.Millisecond)

	cfg.cr.stop()
}

// --------------------------------------------------------------------------
// FUNCIONES DE SUBTESTS

// Se pone en marcha una replica ?? - 3 NODOS RAFT
func (cfg *configDespliegue) soloArranqueYparadaTest1(t *testing.T) {
	//t.Skip("SKIPPED soloArranqueYparadaTest1")

	fmt.Println(t.Name(), ".....................")

	cfg.t = t // Actualizar la estructura de datos de tests para errores

	// Poner en marcha replicas en remoto con un tiempo de espera incluido
	cfg.startDistributedProcesses()

	time.Sleep(6 * time.Second)

	// Comprobar estado replica 0
	cfg.comprobarEstadoRemoto(0, 0, false, -1)

	// Comprobar estado replica 1
	cfg.comprobarEstadoRemoto(1, 0, false, -1)

	// Comprobar estado replica 2
	cfg.comprobarEstadoRemoto(2, 0, false, -1)

	// Parar réplicas almacenamiento en remoto
	cfg.stopDistributedProcesses()

	fmt.Println(".............", t.Name(), "Superado")
}

// Primer lider en marcha - 3 NODOS RAFT
func (cfg *configDespliegue) elegirPrimerLiderTest2(t *testing.T) {
	// t.Skip("SKIPPED ElegirPrimerLiderTest2")

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()

	// Activa proceso de elección en todos los nodos
	time.Sleep(200 * time.Millisecond)
	cfg.activarTimersEnTodosLosNodos()

	// Se ha elegido lider ?
	fmt.Printf("Probando lider en curso\n")
	cfg.pruebaUnLider(3)

	// Parar réplicas alamcenamiento en remoto
	cfg.stopDistributedProcesses() // Parametros

	fmt.Println(".............", t.Name(), "Superado")
}

// Fallo de un primer lider y reeleccion de uno nuevo - 3 NODOS RAFT
func (cfg *configDespliegue) falloAnteriorElegirNuevoLiderTest3(t *testing.T) {
	// t.Skip("SKIPPED FalloAnteriorElegirNuevoLiderTest3")

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()
	// Activa proceso de elección en todos los nodos
	time.Sleep(200 * time.Millisecond)
	cfg.activarTimersEnTodosLosNodos()

	fmt.Printf("Lider inicial\n")
	liderActual := cfg.pruebaUnLider(3)

	// Desconectar lider
	var reply raft.Vacio
	err := cfg.nodosRaft[liderActual].CallTimeout("NodoRaft.ParaNodo",
		raft.Vacio{}, &reply, 10*time.Millisecond)
	check.CheckError(err, "Error en llamada RPC Para nodo")
	cfg.conectados[liderActual] = false

	// Damos un tiempo a que surja nuevo líder
	time.Sleep(1100 * time.Millisecond)
	fmt.Printf("Comprobar nuevo lider\n")
	cfg.pruebaUnLider(3)

	// Parar réplicas almacenamiento en remoto
	cfg.stopDistributedProcesses() //parametros

	fmt.Println(".............", t.Name(), "Superado")
}

// // 3 operaciones comprometidas con situacion estable y sin fallos - 3 NODOS RAFT
func (cfg *configDespliegue) tresOperacionesComprometidasEstable(t *testing.T) {
	// t.Skip("SKIPPED tresOperacionesComprometidasEstable")

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()
	// Activa proceso de elección en todos los nodos
	time.Sleep(400 * time.Millisecond)
	cfg.activarTimersEnTodosLosNodos()

	fmt.Printf("Buscando al Lider inicial\n")
	liderActual := cfg.pruebaUnLider(3)
	fmt.Printf("Líder estable identificado: %d\n", liderActual+1)
	// preparamos datos para someter operaciones al líder
	var reply raft.ResultadoRemoto
	for i := 1; i <= 3; i++ {
		op := raft.TipoOperacion{
			Operacion: "escribir",
			Clave:     fmt.Sprintf("k%d", i),
			Valor:     fmt.Sprintf("v%d", i),
		}
		err := cfg.nodosRaft[liderActual].CallTimeout(
			"NodoRaft.SometerOperacionRaft",
			op,
			&reply,
			30*time.Millisecond,
		)
		check.CheckError(err, "Error RPC SometerOperacion")
	}
	// sometidas tres operaciones al líder estable
	// espera para dar tiempo a resto de nodos replicar
	time.Sleep(2000 * time.Millisecond)
	// recupera situación de los tres nodos
	estadoNodo := make([]raft.EstadoNodo, 3)
	for i := 0; i < 3; i++ {
		err := cfg.nodosRaft[i].CallTimeout(
			"NodoRaft.ObtenerEstadoParaTest",
			raft.Vacio{},
			&estadoNodo[i],
			10*time.Millisecond)
		check.CheckError(err, "Error RPC ObtenerEstadoParaTest")
	}
	// ahora estadoNodo[i]contiene el estado de nodo i
	// Parar réplicas almacenamiento en remoto
	cfg.stopDistributedProcesses()

	// comprueba resultados
	if err := cfg.verificarEstados(estadoNodo, liderActual); err != nil {
		cfg.t.Fatalf("Error en verificación de estado: %s", err)
	}
	fmt.Println(".............", t.Name(), "Superado")
}
func findLeader(estadoNodo []raft.EstadoNodo) int {
	for i := range estadoNodo {
		if estadoNodo[i].Role == raft.Leader {
			return i
		}
	}
	return -1
}
func (cfg *configDespliegue) verificarEstados(estadoNodo []raft.EstadoNodo,
	liderActual int) error {
	for i := 0; i < 3; i++ {
		if i == liderActual && estadoNodo[i].Role != raft.Leader {
			return fmt.Errorf(
				"líder inicial %d pero final %d",
				liderActual+1, findLeader(estadoNodo)+1)
		}
		if estadoNodo[i].CommitIndex != 3 {
			return fmt.Errorf("Replica %d commitIndex=%d, esperado=3", i+1, estadoNodo[i].CommitIndex)
		}
		if estadoNodo[i].LastApplied != 3 {
			return fmt.Errorf("Replica %d lastApplied=%d, esperado=3", i+1, estadoNodo[i].LastApplied)
		}
		if estadoNodo[i].LogLength < 4 {
			return fmt.Errorf("Replica %d LogLength=%d, esperado=3", i+1, estadoNodo[i].LogLength)
		}
	}
	return nil
}

// Se consigue acuerdo a pesar de desconexiones de seguidor -- 3 NODOS RAFT
func (cfg *configDespliegue) AcuerdoApesarDeSeguidor(t *testing.T) {
	// t.Skip("SKIPPED AcuerdoApesarDeSeguidor")
	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()
	time.Sleep(200 * time.Millisecond)
	cfg.activarTimersEnTodosLosNodos()

	// 1. Obtener líder
	lider := cfg.pruebaUnLider(3)
	fmt.Printf("Líder inicial: %d\n", lider)

	// 2. Comprometer 1 operación con todos conectados
	var reply raft.ResultadoRemoto
	op1 := raft.TipoOperacion{
		Operacion: "escribir",
		Clave:     "k1",
		Valor:     "v1",
	}
	err := cfg.nodosRaft[lider].CallTimeout(
		"NodoRaft.SometerOperacionRaft",
		op1,
		&reply,
		30*time.Millisecond,
	)
	check.CheckError(err, "Error RPC SometerOperacion (op1)")

	// Esperar propagación
	time.Sleep(1500 * time.Millisecond)

	// 3. Desconectar un seguidor
	var seguidor int
	if lider == 0 {
		seguidor = 1
	} else {
		seguidor = 0
	}
	fmt.Printf("Desconectando seguidor %d\n", seguidor)

	var vacio raft.Vacio
	err = cfg.nodosRaft[seguidor].CallTimeout(
		"NodoRaft.ParaNodo",
		raft.Vacio{},
		&vacio,
		10*time.Millisecond)
	check.CheckError(err, "Error RPC ParaNodo")
	cfg.conectados[seguidor] = false

	// 4. Comprometer varias operaciones con solo 2 nodos activos (mayoría)
	for i := 2; i <= 4; i++ {
		op := raft.TipoOperacion{
			Operacion: "escribir",
			Clave:     fmt.Sprintf("k%d", i),
			Valor:     fmt.Sprintf("v%d", i),
		}
		err := cfg.nodosRaft[lider].CallTimeout(
			"NodoRaft.SometerOperacionRaft",
			op,
			&reply,
			40*time.Millisecond,
		)
		check.CheckError(err, "Error RPC SometerOperacion (con seguidor desconectado)")
	}

	// Esperar replicación a 1 nodo
	time.Sleep(2000 * time.Millisecond)

	// 5. Reconectar nodo previamente desconectado
	fmt.Printf("Reconectando nodo %d\n", seguidor)
	despliegue.ExecMutipleHosts(
		EXECREPLICACMD+" "+strconv.Itoa(seguidor)+" "+
			rpctimeout.HostPortArrayToString(cfg.nodosRaft),
		[]string{cfg.nodosRaft[seguidor].Host()},
		cfg.cr,
		PRIVKEYFILE,
	)
	cfg.conectados[seguidor] = true
	fmt.Printf("Verifica si en nodo %d está corriendo el server", seguidor)
	time.Sleep(8250 * time.Millisecond)
	// Activar de nuevo timers en ese nodo
	err = cfg.nodosRaft[seguidor].CallTimeout(
		"NodoRaft.ActivarTimers",
		raft.Vacio{},
		&vacio,
		10*time.Millisecond)
	// si error, no abortamos, puede tardar
	if err != nil {
		fmt.Println("Aviso: error activando timers en reconexión:", err)
	}
	time.Sleep(3500 * time.Millisecond)
	// 6. Obtener estado final
	estado := make([]raft.EstadoNodo, 3)
	for i := 0; i < 3; i++ {
		if cfg.conectados[i] {
			err := cfg.nodosRaft[i].CallTimeout(
				"NodoRaft.ObtenerEstadoParaTest",
				raft.Vacio{},
				&estado[i],
				20*time.Millisecond)
			check.CheckError(err, "Error RPC ObtenerEstadoParaTest")
		}
	}

	// 7. Verificar que todos tienen commitIndex >= 4
	for i := 0; i < 3; i++ {
		if !cfg.conectados[i] {
			continue
		}
		if estado[i].CommitIndex < 4 {
			t.Fatalf("Replica %d commitIndex=%d, esperado >=4", i+1, estado[i].CommitIndex)
		}
	}

	fmt.Println(".............", t.Name(), "Superado")

	cfg.stopDistributedProcesses()
}

// NO se consigue acuerdo al desconectarse mayoría de seguidores -- 3 NODOS RAFT
func (cfg *configDespliegue) SinAcuerdoPorFallos(t *testing.T) {
	//t.Skip("SKIPPED SinAcuerdoPorFallos")

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()
	time.Sleep(300 * time.Millisecond)
	cfg.activarTimersEnTodosLosNodos()

	// 1. Obtener líder
	lider := cfg.pruebaUnLider(3)
	fmt.Printf("Líder inicial: %d\n", lider)

	var reply raft.ResultadoRemoto
	var vacio raft.Vacio

	// 2. Comprometer una operación con todos conectados (debe comprometerse)
	op1 := raft.TipoOperacion{Operacion: "escribir", Clave: "k1", Valor: "v1"}
	err := cfg.nodosRaft[lider].CallTimeout("NodoRaft.SometerOperacionRaft",
		op1, &reply, 40*time.Millisecond)
	check.CheckError(err, "Fallo inesperado sometiendo op1")

	time.Sleep(1200 * time.Millisecond)

	// 3. Desconectar 2 seguidores → queda 1 nodo (el líder)
	for i := 0; i < 3; i++ {
		if i != lider {
			fmt.Printf("Desconectando nodo %d\n", i)
			err2 := cfg.nodosRaft[i].CallTimeout("NodoRaft.ParaNodo",
				raft.Vacio{}, &vacio, 10*time.Millisecond)
			check.CheckError(err2, "Error en ParaNodo")
			cfg.conectados[i] = false
		}
	}

	// 4. Intentar someter operaciones sin mayoría → NO deben comprometerse
	for i := 2; i <= 4; i++ {
		op := raft.TipoOperacion{
			Operacion: "escribir",
			Clave:     fmt.Sprintf("k%d", i),
			Valor:     fmt.Sprintf("v%d", i),
		}

		fmt.Printf("Someter op %d sin mayoría\n", i)

		// Aquí NO comprobamos error → es normal que no pueda comprometer
		_ = cfg.nodosRaft[lider].CallTimeout("NodoRaft.SometerOperacionRaft",
			op, &reply, 40*time.Millisecond)
	}

	// 5. Verificar que el commitIndex del líder permanece en 1
	time.Sleep(1200 * time.Millisecond)

	var estLider raft.EstadoNodo
	err = cfg.nodosRaft[lider].CallTimeout("NodoRaft.ObtenerEstadoParaTest",
		raft.Vacio{}, &estLider, 20*time.Millisecond)
	check.CheckError(err, "Error estado líder")

	if estLider.CommitIndex != 1 {
		t.Fatalf("CommitIndex avanzó sin mayoría: %d (esperado 1)", estLider.CommitIndex)
	}

	// 6. Reconectar nodos desconectados
	for i := 0; i < 3; i++ {
		if i != lider {
			fmt.Printf("Reconectando nodo %d\n", i)
			despliegue.ExecMutipleHosts(
				EXECREPLICACMD+" "+strconv.Itoa(i)+" "+
					rpctimeout.HostPortArrayToString(cfg.nodosRaft),
				[]string{cfg.nodosRaft[i].Host()}, cfg.cr, PRIVKEYFILE)

			cfg.conectados[i] = true
			time.Sleep(7 * time.Second)

			err = cfg.nodosRaft[i].CallTimeout("NodoRaft.ActivarTimers",
				raft.Vacio{}, &vacio, 20*time.Millisecond)
			if err != nil {
				fmt.Println("Aviso: error activando timers:", err)
			}
		}
	}

	time.Sleep(3500 * time.Millisecond)

	// 7. Verificar acuerdo final (las 4 operaciones deben comprometerse)
	estado := make([]raft.EstadoNodo, 3)

	for i := 0; i < 3; i++ {
		err = cfg.nodosRaft[i].CallTimeout("NodoRaft.ObtenerEstadoParaTest",
			raft.Vacio{}, &estado[i], 20*time.Millisecond)
		check.CheckError(err, "Error estado final")
	}

	for i := 0; i < 3; i++ {
		if estado[i].CommitIndex < 4 {
			t.Fatalf("Nodo %d CommitIndex=%d, esperado >=4", i, estado[i].CommitIndex)
		}
	}

	fmt.Println(".............", t.Name(), "Superado")

	cfg.stopDistributedProcesses()
}

// Se somete 5 operaciones de forma concurrente -- 3 NODOS RAFT
func (cfg *configDespliegue) SometerConcurrentementeOperaciones(t *testing.T) {

	fmt.Println(t.Name(), ".....................")

	cfg.startDistributedProcesses()
	time.Sleep(300 * time.Millisecond)
	cfg.activarTimersEnTodosLosNodos()

	// 1. Obtener líder
	lider := cfg.pruebaUnLider(3)
	fmt.Printf("Líder inicial: %d\n", lider)

	// 2. Sometemos una operación de estabilización
	var reply raft.ResultadoRemoto
	op1 := raft.TipoOperacion{Operacion: "escribir", Clave: "k1", Valor: "v1"}

	err := cfg.nodosRaft[lider].CallTimeout("NodoRaft.SometerOperacionRaft",
		op1, &reply, 40*time.Millisecond)
	check.CheckError(err, "Error inicial")

	time.Sleep(1200 * time.Millisecond)

	// 3. Someter 5 operaciones concurrentes
	for i := 2; i <= 6; i++ {
		go func(i int) {
			var r raft.ResultadoRemoto
			op := raft.TipoOperacion{
				Operacion: "escribir",
				Clave:     fmt.Sprintf("k%d", i),
				Valor:     fmt.Sprintf("v%d", i),
			}
			_ = cfg.nodosRaft[lider].CallTimeout("NodoRaft.SometerOperacionRaft",
				op, &r, 50*time.Millisecond)
		}(i)
	}

	time.Sleep(3000 * time.Millisecond)

	// 4. Comprobar estados
	estado := make([]raft.EstadoNodo, 3)

	for i := 0; i < 3; i++ {
		err = cfg.nodosRaft[i].CallTimeout("NodoRaft.ObtenerEstadoParaTest",
			raft.Vacio{}, &estado[i], 40*time.Millisecond)
		check.CheckError(err, "Error obteniendo estado")
	}

	// --- Comprobar coherencia del término ---
	if !(estado[0].Term == estado[1].Term && estado[1].Term == estado[2].Term) {
		t.Fatalf("Términos distintos: %v %v %v", estado[0].Term, estado[1].Term, estado[2].Term)
	}

	// --- Comprobar progreso ---
	for i := 0; i < 3; i++ {
		if estado[i].CommitIndex < 6 {
			t.Fatalf("Nodo %d CommitIndex=%d, esperado >=6",
				i, estado[i].CommitIndex)
		}
	}

	fmt.Println(".............", t.Name(), "Superado")

	cfg.stopDistributedProcesses()
}

// --------------------------------------------------------------------------
// FUNCIONES DE APOYO
// Comprobar que hay un solo lider
// probar varias veces si se necesitan reelecciones
func (cfg *configDespliegue) pruebaUnLider(numreplicas int) int {
	for iters := 0; iters < 10; iters++ {
		time.Sleep(2000 * time.Millisecond)
		mapaLideres := make(map[int][]int) // para almacenar en cada mandato la lista de líderes
		for i := 0; i < numreplicas; i++ {
			if cfg.conectados[i] {
				if _, mandato, eslider, _ := cfg.obtenerEstadoRemoto(i); eslider {
					mapaLideres[mandato] = append(mapaLideres[mandato], i)
				}
			}
		}

		ultimoMandatoConLider := -1
		for mandato, lideres := range mapaLideres {
			if len(lideres) > 1 {
				cfg.t.Fatalf("mandato %d tiene %d (>1) lideres",
					mandato, len(lideres))
			}
			if mandato > ultimoMandatoConLider {
				ultimoMandatoConLider = mandato
			}
		}

		if len(mapaLideres) != 0 {

			return mapaLideres[ultimoMandatoConLider][0] // Termina

		}
	}
	cfg.t.Fatalf("un lider esperado, ninguno obtenido")

	return -1 // Termina
}

func (cfg *configDespliegue) obtenerEstadoRemoto(
	indiceNodo int) (int, int, bool, int) {
	var reply raft.EstadoRemoto
	err := cfg.nodosRaft[indiceNodo].CallTimeout("NodoRaft.ObtenerEstadoNodo",
		raft.Vacio{}, &reply, 10*time.Millisecond)
	check.CheckError(err, "Error en llamada RPC ObtenerEstadoRemoto")

	return reply.IdNodo, reply.Mandato, reply.EsLider, reply.IdLider
}

// start  gestor de vistas; mapa de replicas y maquinas donde ubicarlos;
// y lista clientes (host:puerto)
func (cfg *configDespliegue) startDistributedProcesses() {
	//cfg.t.Log("Before starting following distributed processes: ", cfg.nodosRaft)

	for i, endPoint := range cfg.nodosRaft {
		cfg.conectados[i] = true // control de qué nodos están activos
		despliegue.ExecMutipleHosts(EXECREPLICACMD+
			" "+strconv.Itoa(i)+" "+
			rpctimeout.HostPortArrayToString(cfg.nodosRaft),
			[]string{endPoint.Host()}, cfg.cr, PRIVKEYFILE)

		// dar tiempo para se establezcan las replicas
		//time.Sleep(2000 * time.Millisecond)
	}

	// aproximadamente 500 ms para cada arranque por ssh en portatil
	time.Sleep(2000 * time.Millisecond)
}

func (cfg *configDespliegue) stopDistributedProcesses() {
	var reply raft.Vacio

	for i, endPoint := range cfg.nodosRaft {
		if cfg.conectados[i] {
			err := endPoint.CallTimeout("NodoRaft.ParaNodo",
				raft.Vacio{}, &reply, 10*time.Millisecond)
			check.CheckError(err, "Error en llamada RPC Para nodo")
			cfg.conectados[i] = false // para poder llamar a
			// stopDistributedProcesses desde stop()
		}
	}
}

func (cfg *configDespliegue) activarTimersEnTodosLosNodos() {
	var reply raft.Vacio
	for i := range cfg.nodosRaft {
		err := cfg.nodosRaft[i].CallTimeout(
			"NodoRaft.ActivarTimers",
			raft.Vacio{},
			&reply,
			250*time.Millisecond)
		check.CheckError(err, "Error en llamada RPC ActivarTimers")
	}
}

// Comprobar estado remoto de un nodo con respecto a un estado prefijado
func (cfg *configDespliegue) comprobarEstadoRemoto(idNodoDeseado int,
	mandatoDeseado int, esLiderDeseado bool, IdLiderDeseado int) {
	idNodo, mandato, esLider, idLider := cfg.obtenerEstadoRemoto(idNodoDeseado)

	//cfg.t.Log("Estado replica 0: ", idNodo, mandato, esLider, idLider, "\n")

	if idNodo != idNodoDeseado || mandato != mandatoDeseado ||
		esLider != esLiderDeseado || idLider != IdLiderDeseado {
		cfg.t.Fatalf("Estado incorrecto en replica %d en subtest %s; idNodo recibido: %d, term: %d, esLider? %v, idLider= %d\n",
			idNodoDeseado, cfg.t.Name(), idNodo, mandato, esLider, idLider)
	}

}
