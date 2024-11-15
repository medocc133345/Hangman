package main

import (
	crand "crypto/rand" // Alias pour crypto/rand
	"encoding/hex"
	"encoding/json"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Game représente l'état d'une partie en cours ou terminée
type Game struct {
	Username       string
	Difficulty     string
	Category       string
	Word           string
	GuessedLetters []string
	AttemptsLeft   int
	Status         string // "ongoing", "won", "lost"
	Message        string // Message de feedback
	MessageType    string // "success" ou "error"
	CreatedAt      time.Time
	HintsUsed      int    // Nombre d'indices utilisés
	CSRFToken      string // Token CSRF
	Theme          string // Thème choisi
}

// Score représente une entrée dans le leaderboard
type Score struct {
	Username    string `json:"username"`
	Difficulty  string `json:"difficulty"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	Word        string `json:"word"`
	HintsUsed   int    `json:"hints_used"`
	Timestamp   int64  `json:"timestamp"`
}

// Variables globales
var (
	templates = template.Must(template.New("").Funcs(template.FuncMap{
		"displayWord": displayWord,
		"title":       strings.Title, // Fonction pour capitaliser la première lettre
		"timeFormat":  func(timestamp int64) string {
			t := time.Unix(timestamp, 0)
			return t.Format("02/01/2006 15:04:05")
		},
	}).ParseGlob("templates/*.html"))

	games           = make(map[string]*Game) // Map pour stocker les parties en cours
	gamesMutex      sync.Mutex                // Mutex pour sécuriser l'accès concurrent
	wordsByCategory = loadWords()             // Mots chargés depuis les fichiers
	scoreFilePath   = "scores/scores.json"    // Chemin vers le fichier des scores
	sessionExpiration = 30 * time.Minute      // Expiration des sessions
	maxHints        = 2                       // Nombre maximum d'indices
)

func main() {
	// Initialiser la graine aléatoire pour math/rand
	rand.Seed(time.Now().UnixNano())

	// Lancer la goroutine de nettoyage des sessions
	go cleanupSessions()

	// Configurer les routes
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/game", gameHandler)
	http.HandleFunc("/end", endHandler)
	http.HandleFunc("/scores", scoresHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Println("Serveur démarré sur http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// Fonction pour charger les mots depuis les fichiers
func loadWords() map[string]map[string][]string {
	categories := []string{"animals", "technology", "countries", "random"}
	difficulties := []string{"easy", "medium", "hard"}

	words := make(map[string]map[string][]string)

	for _, category := range categories {
		words[category] = make(map[string][]string)
		for _, difficulty := range difficulties {
			filePath := filepath.Join("words", category+"_"+difficulty+".txt")
			log.Printf("Chargement des mots depuis : %s", filePath)
			data, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("Erreur de lecture du fichier %s: %v\n", filePath, err)
				words[category][difficulty] = []string{}
				continue
			}
			lines := strings.Split(string(data), "\n")
			var categoryWords []string
			for _, line := range lines {
				word := strings.TrimSpace(line)
				if word != "" {
					categoryWords = append(categoryWords, word)
				}
			}
			words[category][difficulty] = categoryWords
		}
	}

	return words
}

// Handler pour la page d'accueil
func indexHandler(w http.ResponseWriter, r *http.Request) {
	// Si une partie est en cours, rediriger vers la page de jeu
	sessionID := getSessionID(r)
	if sessionID != "" {
		gamesMutex.Lock()
		game, exists := games[sessionID]
		gamesMutex.Unlock()
		if exists && game.Status == "ongoing" {
			http.Redirect(w, r, "/game", http.StatusSeeOther)
			return
		}
	}

	// Gérer le formulaire de démarrage de partie
	if r.Method == http.MethodPost {
		username := strings.TrimSpace(r.FormValue("username"))
		difficulty := r.FormValue("difficulty")
		category := r.FormValue("category")
		theme := r.FormValue("theme")

		if username == "" || difficulty == "" || category == "" || theme == "" {
			http.Error(w, "Tous les champs sont requis.", http.StatusBadRequest)
			return
		}

		word := getRandomWord(difficulty, category)
		if word == "erreur" {
			http.Error(w, "Aucun mot disponible pour cette catégorie ou ce niveau de difficulté.", http.StatusInternalServerError)
			return
		}

		game := &Game{
			Username:       username,
			Difficulty:     difficulty,
			Category:       category,
			Word:           strings.ToLower(word),
			GuessedLetters: []string{},
			AttemptsLeft:   6,
			Status:         "ongoing",
			CreatedAt:      time.Now(),
			HintsUsed:      0,
			Theme:          theme,
			CSRFToken:      generateCSRFToken(),
		}

		sessionID = generateSessionID()
		gamesMutex.Lock()
		games[sessionID] = game
		gamesMutex.Unlock()

		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			// Secure:   true, // Décommentez si vous utilisez HTTPS
		})

		http.Redirect(w, r, "/game", http.StatusSeeOther)
		return
	}

	// Afficher la page d'accueil
	err := templates.ExecuteTemplate(w, "index.html", nil)
	if err != nil {
		http.Error(w, "Erreur lors du rendu de la page.", http.StatusInternalServerError)
	}
}

// Handler pour la page de jeu
func gameHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := getSessionID(r)
	if sessionID == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	gamesMutex.Lock()
	game, exists := games[sessionID]
	gamesMutex.Unlock()

	if !exists {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Si la partie est terminée, rediriger vers la page de fin
	if game.Status != "ongoing" {
		http.Redirect(w, r, "/end", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodPost {
		// Vérifier le token CSRF
		csrfToken := r.FormValue("csrf_token")
		if csrfToken != game.CSRFToken {
			http.Error(w, "Invalid CSRF Token", http.StatusForbidden)
			return
		}

		action := r.FormValue("action")
		if action == "hint" {
			if game.HintsUsed >= maxHints {
				game.Message = "Vous avez atteint le nombre maximum d'indices."
				game.MessageType = "error"
				goto render
			}
			if game.AttemptsLeft <= 0 {
				game.Message = "Vous n'avez plus de tentatives pour demander un indice."
				game.MessageType = "error"
				goto render
			}
			provideHint(game)
			game.AttemptsLeft-- // Déduire une tentative pour utiliser un indice

			// Vérifier si le jeu est gagné ou perdu
			if allLettersGuessed(game.Word, game.GuessedLetters) {
				game.Status = "won"
				game.Message = "Félicitations ! Vous avez deviné toutes les lettres."
				game.MessageType = "success"
			}
			if game.AttemptsLeft <= 0 && game.Status != "won" {
				game.Status = "lost"
				game.Message = "Vous avez perdu. Le mot était : " + game.Word
				game.MessageType = "error"
			}

			// Enregistrer le score si la partie est terminée
			if game.Status != "ongoing" {
				saveScore(game)
			}

			gamesMutex.Lock()
			games[sessionID] = game
			gamesMutex.Unlock()

			if game.Status != "ongoing" {
				http.Redirect(w, r, "/end", http.StatusSeeOther)
				return
			}

			goto render
		}

		// Gestion des devinettes
		guess := strings.TrimSpace(strings.ToLower(r.FormValue("guess")))
		if guess == "" || !isAlpha(guess) {
			game.Message = "Veuillez entrer une lettre ou un mot valide."
			game.MessageType = "error"
			goto render
		}

		// Vérifier si c'est une lettre ou un mot
		if len(guess) == 1 {
			// Lettre
			if contains(game.GuessedLetters, guess) {
				game.Message = "Vous avez déjà essayé cette lettre."
				game.MessageType = "error"
			} else {
				game.GuessedLetters = append(game.GuessedLetters, guess)
				if strings.Contains(game.Word, guess) {
					game.Message = "Bonne réponse !"
					game.MessageType = "success"
				} else {
					game.AttemptsLeft--
					game.Message = "Mauvaise réponse."
					game.MessageType = "error"
				}
			}
		} else {
			// Mot
			if guess == game.Word {
				game.Status = "won"
				game.Message = "Félicitations ! Vous avez deviné le mot."
				game.MessageType = "success"
			} else {
				game.AttemptsLeft--
				game.Message = "Mauvaise réponse."
				game.MessageType = "error"
			}
		}

		// Vérifier si le joueur a gagné
		if allLettersGuessed(game.Word, game.GuessedLetters) {
			game.Status = "won"
			game.Message = "Félicitations ! Vous avez deviné toutes les lettres."
			game.MessageType = "success"
		}

		// Vérifier si le joueur a perdu
		if game.AttemptsLeft <= 0 && game.Status != "won" {
			game.Status = "lost"
			game.Message = "Vous avez perdu. Le mot était : " + game.Word
			game.MessageType = "error"
		}

		// Enregistrer le score si la partie est terminée
		if game.Status != "ongoing" {
			saveScore(game)
		}

		gamesMutex.Lock()
		games[sessionID] = game
		gamesMutex.Unlock()

		if game.Status != "ongoing" {
			http.Redirect(w, r, "/end", http.StatusSeeOther)
			return
		}

	}

render:
	// Afficher la page de jeu avec l'état actuel
	err := templates.ExecuteTemplate(w, "game.html", game)
	if err != nil {
		http.Error(w, "Erreur lors du rendu de la page.", http.StatusInternalServerError)
	}
}

// Handler pour la page de fin de partie
func endHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := getSessionID(r)
	if sessionID == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	gamesMutex.Lock()
	game, exists := games[sessionID]
	gamesMutex.Unlock()

	if !exists {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Si la partie est toujours en cours, rediriger vers la page de jeu
	if game.Status == "ongoing" {
		http.Redirect(w, r, "/game", http.StatusSeeOther)
		return
	}

	// Afficher la page de fin de partie
	err := templates.ExecuteTemplate(w, "end.html", game)
	if err != nil {
		http.Error(w, "Erreur lors du rendu de la page.", http.StatusInternalServerError)
	}
}

// Handler pour la page des scores
func scoresHandler(w http.ResponseWriter, r *http.Request) {
	// Lire les scores depuis le fichier
	scoresData, err := os.ReadFile(scoreFilePath)
	if err != nil {
		http.Error(w, "Impossible de lire les scores.", http.StatusInternalServerError)
		return
	}

	var scores []Score
	lines := strings.Split(string(scoresData), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var score Score
		if err := json.Unmarshal([]byte(line), &score); err != nil {
			log.Println("Erreur de parsing du score:", err)
			continue
		}
		scores = append(scores, score)
	}

	// Trier les scores par date décroissante
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Timestamp > scores[j].Timestamp
	})

	data := struct {
		Scores []Score
	}{
		Scores: scores,
	}

	// Afficher la page des scores
	err = templates.ExecuteTemplate(w, "scores.html", data)
	if err != nil {
		http.Error(w, "Erreur lors du rendu de la page.", http.StatusInternalServerError)
	}
}

// Sélectionne un mot aléatoire basé sur le niveau de difficulté et la catégorie
func getRandomWord(difficulty, category string) string {
	categoryWords, exists := wordsByCategory[category]
	if !exists {
		return "erreur"
	}
	words, exists := categoryWords[difficulty]
	if !exists || len(words) == 0 {
		return "erreur"
	}
	return words[rand.Intn(len(words))]
}

// Génère un ID de session unique basé sur des bytes aléatoires
func generateSessionID() string {
	bytes := make([]byte, 16)
	if _, err := crand.Read(bytes); err != nil {
		log.Println("Erreur lors de la génération du session ID:", err)
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(bytes)
}

// Génère un token CSRF
func generateCSRFToken() string {
	bytes := make([]byte, 16)
	if _, err := crand.Read(bytes); err != nil {
		log.Println("Erreur lors de la génération du CSRF Token:", err)
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(bytes)
}

// Récupère l'ID de session depuis les cookies
func getSessionID(r *http.Request) string {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return ""
	}
	return cookie.Value
}

// Vérifie si une chaîne contient uniquement des lettres et des espaces
func isAlpha(s string) bool {
	for _, c := range s {
		if !('a' <= c && c <= 'z') && !('A' <= c && c <= 'Z') && c != ' ' {
			return false
		}
	}
	return true
}

// Vérifie si un slice contient un élément spécifique
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Vérifie si toutes les lettres du mot ont été devinées
func allLettersGuessed(word string, guessed []string) bool {
	for _, c := range word {
		if !contains(guessed, string(c)) {
			return false
		}
	}
	return true
}

// Enregistre le score de la partie dans le fichier des scores
func saveScore(game *Game) {
	score := Score{
		Username:    game.Username,
		Difficulty:  game.Difficulty,
		Category:    game.Category,
		Status:      game.Status,
		Word:        game.Word,
		HintsUsed:   game.HintsUsed,
		Timestamp:   time.Now().Unix(),
	}

	data, err := json.Marshal(score)
	if err != nil {
		log.Println("Erreur de marshalling du score:", err)
		return
	}

	f, err := os.OpenFile(scoreFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println("Erreur d'ouverture du fichier de scores:", err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(string(data) + "\n"); err != nil {
		log.Println("Erreur d'écriture dans le fichier de scores:", err)
	}
}

// Fonction personnalisée pour afficher le mot avec les lettres devinées
func displayWord(word string, guessed []string) string {
	display := ""
	for _, c := range word {
		if contains(guessed, string(c)) {
			display += string(c) + " "
		} else {
			display += "_ "
		}
	}
	return strings.TrimSpace(display) // Supprime l'espace final
}

// Fournit un indice en révélant une lettre non devinée
func provideHint(game *Game) {
	for _, c := range game.Word {
		letter := string(c)
		if !contains(game.GuessedLetters, letter) {
			game.GuessedLetters = append(game.GuessedLetters, letter)
			game.HintsUsed++
			game.Message = "Indice : Une lettre a été révélée."
			game.MessageType = "success"
			break
		}
	}
}

// Fonction de nettoyage des sessions expirées
func cleanupSessions() {
	for {
		time.Sleep(10 * time.Minute)
		gamesMutex.Lock()
		for id, game := range games {
			if time.Since(game.CreatedAt) > sessionExpiration {
				delete(games, id)
			}
		}
		gamesMutex.Unlock()
	}
}
