package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
)

// Estrutura para o corpo da requisição de criação de chave
type CreateKeyRequest struct {
	Name string `json:"name"`
}

// Estrutura para a resposta da criação de chave
type CreateKeyResponse struct {
	Name    string `json:"name"`
	Key     string `json:"key"` // A chave em texto plano é retornada APENAS uma vez
	Message string `json:"message"`
}

// healthHandler é um simples endpoint de verificação de saúde
func (a *App) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("erro ao escrever resposta de health: %v", err)
	}
}

// validateKeyHandler verifica se uma chave de API (enviada via Header) é válida
func (a *App) validateKeyHandler(w http.ResponseWriter, r *http.Request) {
	if a.faultInjected.Load() {
		log.Printf("ERRO: falha injetada ativa, respondendo 500 em /validate")
		http.Error(w, "Falha interna simulada", http.StatusInternalServerError)
		return
	}

	// Extrai a chave do header "Authorization: Bearer <key>"
	authHeader := r.Header.Get("Authorization")
	keyString := strings.TrimPrefix(authHeader, "Bearer ")

	if keyString == "" {
		http.Error(w, "Authorization header não encontrado", http.StatusUnauthorized)
		return
	}

	// Calcula o hash da chave recebida
	keyHash := hashAPIKey(keyString)

	// Verifica se o hash existe no banco de dados
	var id int
	err := a.DB.QueryRowContext(r.Context(), "SELECT id FROM api_keys WHERE key_hash = $1 AND is_active = true", keyHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("Chave inválida ou inativa (hash: %s...)", keyHash[:6])
		http.Error(w, "Chave de API inválida ou inativa", http.StatusUnauthorized)
		return
	}
	if err != nil {
		// Falha de infraestrutura precisa aparecer como 5xx, senão o alerta não enxerga.
		log.Printf("ERRO ao consultar a chave no banco: %v", err)
		http.Error(w, "Erro interno ao validar a chave", http.StatusInternalServerError)
		return
	}

	// Chave válida
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Chave válida"}); err != nil {
		log.Printf("erro ao escrever resposta de validação: %v", err)
	}
}

// createKeyHandler cria uma nova chave de API
func (a *App) createKeyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req CreateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Corpo da requisição inválido", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "O campo 'name' é obrigatório", http.StatusBadRequest)
		return
	}

	// Gera uma nova chave e seu hash
	newKey, err := generateAPIKey()
	if err != nil {
		http.Error(w, "Erro ao gerar a chave", http.StatusInternalServerError)
		return
	}
	newKeyHash := hashAPIKey(newKey)

	// Salva o hash no banco de dados
	var newID int
	err = a.DB.QueryRowContext(
		r.Context(),
		"INSERT INTO api_keys (name, key_hash) VALUES ($1, $2) RETURNING id",
		req.Name, newKeyHash,
	).Scan(&newID)

	if err != nil {
		log.Printf("Erro ao salvar a chave no banco: %v", err)
		http.Error(w, "Erro ao salvar a chave", http.StatusInternalServerError)
		return
	}

	log.Printf("Nova chave criada com sucesso (ID: %d, Name: %s)", newID, req.Name)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(CreateKeyResponse{
		Name:    req.Name,
		Key:     newKey, // Retorna a chave em texto plano pela última vez
		Message: "Guarde esta chave com segurança! Você não poderá vê-la novamente.",
	}); err != nil {
		log.Printf("erro ao escrever resposta de criação de chave: %v", err)
	}
}

// chaosHandler liga (POST) ou desliga (DELETE) a falha simulada neste Pod.
func (a *App) chaosHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.faultInjected.Store(true)
	case http.MethodDelete:
		a.faultInjected.Store(false)
	default:
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}
	log.Printf("Injeção de falha alterada: ativa=%t", a.faultInjected.Load())
	w.WriteHeader(http.StatusNoContent)
}

// --- Middleware ---

// masterKeyAuthMiddleware protege endpoints que só podem ser acessados com a MASTER_KEY
func (a *App) masterKeyAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		keyString := strings.TrimPrefix(authHeader, "Bearer ")

		if keyString != a.MasterKey {
			http.Error(w, "Acesso não autorizado", http.StatusForbidden)
			return
		}
		// Se a chave for válida, continua para o handler principal
		next.ServeHTTP(w, r)
	})
}
