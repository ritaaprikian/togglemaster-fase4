package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/otel/attribute"
)

// App struct (para injeção de dependência)
type App struct {
	DB        *sql.DB
	MasterKey string
	// faultInjected fica só na memória do Pod: um restart corrige a falha simulada.
	faultInjected atomic.Bool
}

func main() {
	// Carrega o .env para desenvolvimento local. Em produção, isso não fará nada.
	_ = godotenv.Load()

	shutdownTelemetry, err := initTelemetry(context.Background(), "auth-service")
	if err != nil {
		log.Fatalf("Não foi possível iniciar a telemetria: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(ctx); err != nil {
			log.Printf("erro ao encerrar a telemetria: %v", err)
		}
	}()

	// --- Configuração ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8001" // Porta padrão
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL deve ser definida")
	}

	masterKey := os.Getenv("MASTER_KEY")
	if masterKey == "" {
		log.Fatal("MASTER_KEY deve ser definida")
	}

	// --- Conexão com o Banco ---
	db, err := connectDB(databaseURL)
	if err != nil {
		log.Fatalf("Não foi possível conectar ao banco de dados: %v", err)
	}
	defer func() { _ = db.Close() }()

	app := &App{
		DB:        db,
		MasterKey: masterKey,
	}

	// --- Rotas da API ---
	mux := http.NewServeMux()
	mux.HandleFunc("/health", app.healthHandler)

	// Endpoint público para validar uma chave
	mux.HandleFunc("/validate", app.validateKeyHandler)

	// Endpoints de "admin" para criar/gerenciar chaves
	// Eles são protegidos pelo middleware de autenticação
	mux.Handle("/admin/keys", app.masterKeyAuthMiddleware(http.HandlerFunc(app.createKeyHandler)))

	// Injeção de falha para a demonstração de incidente (desligada por padrão)
	if os.Getenv("CHAOS_ENABLED") == "true" {
		mux.Handle("/admin/chaos", app.masterKeyAuthMiddleware(http.HandlerFunc(app.chaosHandler)))
		log.Println("Atenção: injeção de falha habilitada em /admin/chaos.")
	}

	log.Printf("Serviço de Autenticação (Go) rodando na porta %s", port)
	if err := http.ListenAndServe(":"+port, instrumentHTTP(mux, "auth-service")); err != nil {
		log.Fatal(err)
	}
}

// connectDB inicializa e testa a conexão com o PostgreSQL
func connectDB(databaseURL string) (*sql.DB, error) {
	db, err := otelsql.Open("pgx", databaseURL,
		otelsql.WithAttributes(attribute.String("db.system", "postgresql")),
		otelsql.WithSpanOptions(otelsql.SpanOptions{Ping: false, OmitRows: true}),
	)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	log.Println("Conectado ao PostgreSQL com sucesso!")
	return db, nil
}
