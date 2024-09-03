package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type Transacao struct {
	ID          int32     `json:"id"`
	ClienteID   int32     `json:"cliente_id"`
	Valor       int32     `json:"valor"`
	Tipo        string    `json:"tipo"`
	Descricao   string    `json:"descricao"`
	RealizadaEm time.Time `json:"realizada_em"`
}

type Saldo struct {
	Total       int32     `json:"total"`
	DataExtrato time.Time `json:"data_extrato"`
	Limite      int32     `json:"limite"`
}

type Resultado struct {
	Saldo             Saldo       `json:"saldo"`
	UltimasTransacoes []Transacao `json:"ultimas_transacoes"`
}

type Cliente struct {
	ID     int32 `json:"id"`
	Saldo  int32 `json:"saldo"`
	Limite int32 `json:"limite"`
}

type apiConfig struct {
	DB *pgxpool.Pool
}

func Config() *pgxpool.Config {
	godotenv.Load()

	const defaultMaxConns = int32(50)
	const defaultMinConns = int32(49)
	const defaultMaxConnLifetime = time.Hour
	const defaultMaxConnIdleTime = time.Minute * 30
	const defaultHealthCheckPeriod = time.Minute * 10

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL must be set")
	}

	dbConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatal("Failed to create a config, error: ", err)
	}

	dbConfig.MaxConns = defaultMaxConns
	dbConfig.MinConns = defaultMinConns
	dbConfig.MaxConnLifetime = defaultMaxConnLifetime
	dbConfig.MaxConnIdleTime = defaultMaxConnIdleTime
	dbConfig.HealthCheckPeriod = defaultHealthCheckPeriod

	return dbConfig
}

func main() {
	godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT must be set")
	}

	connPool, err := pgxpool.NewWithConfig(context.Background(), Config())
	if err != nil {
		log.Fatal("Error while creating connection to the database!!")
	}
	defer connPool.Close()

	apiCfg := apiConfig{
		DB: connPool,
	}

	router := chi.NewRouter()
	router.Post("/clientes/{id}/transacoes", apiCfg.handlerCreateTransacao)
	router.Get("/clientes/{id}/extrato", apiCfg.handlerGetClientExpenses)

	srv := &http.Server{
		Handler: router,
		Addr:    ":" + port,
	}

	fmt.Println("running on http://localhost:" + port)
	log.Fatal(srv.ListenAndServe())
}

func (apiCfg *apiConfig) handlerCreateTransacao(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	type request struct {
		Valor     int32  `json:"valor"`
		Tipo      string `json:"tipo"`
		Descricao string `json:"descricao"`
	}

	type response struct {
		Limite int `json:"limite"`
		Saldo  int `json:"saldo"`
	}

	params := request{}
	err := json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusUnprocessableEntity, "error parsing json")
		return
	}

	if params.Valor <= 0 {
		respondWithError(w, http.StatusUnprocessableEntity, "Valor must be positive")
		return
	}
	if params.Tipo != "c" && params.Tipo != "d" {
		respondWithError(w, http.StatusUnprocessableEntity, "Tipo must be 'c' or 'd'")
		return
	}
	if len(params.Descricao) > 10 {
		respondWithError(w, http.StatusUnprocessableEntity, "Descricao must be less than 10 characters")
		return
	}

	userIDStr := chi.URLParam(r, "id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		respondWithError(w, http.StatusUnprocessableEntity, "Invalid user ID")
		return
	}

	if userID < 1 || userID > 5 {
		respondWithError(w, http.StatusNotFound, "User not found")
		return
	}

	var limit int
	var success bool
	var newBalance int
	if params.Tipo == "c" {
		err = apiCfg.DB.QueryRow(r.Context(), "SELECT * from credito_cliente($1, $2, $3)", userID, params.Valor, params.Descricao).Scan(&limit, &success, &newBalance)
		if err != nil {
			respondWithError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	} else {
		err = apiCfg.DB.QueryRow(r.Context(), "SELECT * from debito_cliente($1, $2, $3)", userID, params.Valor, params.Descricao).Scan(&limit, &success, &newBalance)
		if err != nil || !success {
			respondWithError(w, http.StatusUnprocessableEntity, "erro")
			return
		}
	}

	resp := response{
		Limite: limit,
		Saldo:  newBalance,
	}
	respondWithJSON(w, http.StatusOK, resp)
}

func (apiCfg *apiConfig) handlerGetClientExpenses(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "id")
	userID, err := strconv.Atoi(userIDStr)
	if userID < 1 || userID > 5 {
		respondWithError(w, http.StatusNotFound, "User not found")
		return
	}

	if err != nil {
		respondWithError(w, http.StatusUnprocessableEntity, "Invalid user ID")
		return
	}

	query := `
	SELECT 
			c.id, c.saldo, c.limite, 
			t.id, t.cliente_id, t.valor, t.tipo, t.descricao, t.realizada_em
	FROM 
			clientes c
	LEFT JOIN 
			transacoes t 
	ON 
			c.id = t.cliente_id
	WHERE 
			c.id = $1
	ORDER BY 
			t.realizada_em DESC
	LIMIT 10`

	rows, err := apiCfg.DB.Query(r.Context(), query, userID)
	if err != nil {
		respondWithError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	defer rows.Close()

	var user Cliente
	var transacoes []Transacao

	for rows.Next() {
		var (
			transacaoID          *int32
			transacaoClienteID   *int32
			transacaoValor       *int32
			transacaoTipo        *string
			transacaoDescricao   *string
			transacaoRealizadaEm *time.Time
		)

		err := rows.Scan(&user.ID, &user.Saldo, &user.Limite,
			&transacaoID, &transacaoClienteID, &transacaoValor,
			&transacaoTipo, &transacaoDescricao, &transacaoRealizadaEm)
		if err != nil {
			respondWithError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		if transacaoID != nil {
			transacoes = append(transacoes, Transacao{
				ID:          *transacaoID,
				ClienteID:   *transacaoClienteID,
				Valor:       *transacaoValor,
				Tipo:        *transacaoTipo,
				Descricao:   *transacaoDescricao,
				RealizadaEm: *transacaoRealizadaEm,
			})
		}
	}

	if rows.Err() != nil {
		respondWithError(w, http.StatusUnprocessableEntity, rows.Err().Error())
		return
	}

	result := Resultado{
		Saldo: Saldo{
			Total:       user.Saldo,
			DataExtrato: time.Now().UTC(),
			Limite:      user.Limite,
		},
		UltimasTransacoes: transacoes,
	}
	respondWithJSON(w, http.StatusOK, result)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("failed to marshal json %v\n", payload)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	if code > 499 {
		log.Println("Responding with 5XX error: ", message)
	}
	type errResponse struct {
		Error string `json:"error"`
	}
	respondWithJSON(w, code, errResponse{Error: message})
}
