package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

func chargerMots(nomFichier string) ([]string, error) {
	file, err := os.Open(nomFichier)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var mots []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		mots = append(mots, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return mots, nil
}

func filtrerMotsParNiveau(mots []string, niveau string) []string {
	var motsFiltres []string
	for _, mot := range mots {
		if niveau == "1" && len(mot) >= 3 && len(mot) <= 5 {
			motsFiltres = append(motsFiltres, mot)
		} else if niveau == "2" && len(mot) >= 6 {
			motsFiltres = append(motsFiltres, mot)
		}
	}
	return motsFiltres
}

func afficherMot(mot string, lettresDevinees map[rune]bool) string {
	resultat := ""
	for _, lettre := range mot {
		if lettresDevinees[lettre] {
			resultat += string(lettre) + " "
		} else {
			resultat += "_ "
		}
	}
	return resultat
}

func jouerPendu(niveau string) {

	mots, err := chargerMots("mots.txt")
	if err != nil {
		fmt.Println("Erreur lors du chargement des mots:", err)
		return
	}

	motsFiltres := filtrerMotsParNiveau(mots, niveau)
	if len(motsFiltres) == 0 {
		fmt.Println("Aucun mot trouvé pour ce niveau de difficulté.")
		return
	}

	rand.Seed(time.Now().UnixNano())
	motADeviner := motsFiltres[rand.Intn(len(motsFiltres))]

	lettresDevinees := make(map[rune]bool)
	nbErreurs := 0
	nbEssaisMax := 6

	var limiteDeTemps time.Duration
	if niveau == "1" {
		limiteDeTemps = 1*time.Minute + 30*time.Second 
	} else if niveau == "2" {
		limiteDeTemps = 3 * time.Minute 
	}

	debut := time.Now()

	for {
		
		tempsEcoule := time.Since(debut)
		tempsRestant := limiteDeTemps - tempsEcoule
		if tempsRestant <= 0 {
			fmt.Println("\nTemps écoulé ! Vous avez perdu.")
			fmt.Printf("Le mot était : %s\n", motADeviner)
			break
		}
		fmt.Printf("\nTemps restant : %.0f secondes\n", tempsRestant.Seconds())

		fmt.Println("\nMot à deviner : ", afficherMot(motADeviner, lettresDevinees))
		fmt.Printf("Nombre d'erreurs : %d/%d\n", nbErreurs, nbEssaisMax)

		fmt.Print("Devinez une lettre : ")
		var lettre string
		fmt.Scan(&lettre)

		lettreRune := rune(strings.ToLower(lettre)[0])
		if strings.ContainsRune(motADeviner, lettreRune) {
			lettresDevinees[lettreRune] = true
			fmt.Println("Bonne réponse !")
		} else {
			nbErreurs++
			fmt.Println("Mauvaise réponse...")
		}

		gagne := true
		for _, lettre := range motADeviner {
			if !lettresDevinees[lettre] {
				gagne = false
				break
			}
		}

		if gagne {
			fmt.Println("\nFélicitations ! Vous avez deviné le mot :", motADeviner)
			break
		}

		if nbErreurs >= nbEssaisMax {
			fmt.Println("\nVous avez perdu ! Le mot était :", motADeviner)
			break
		}
	}

	fin := time.Now()
	duree := fin.Sub(debut)

	fmt.Printf("Temps écoulé : %.2f secondes\n", duree.Seconds())
}

func afficherRegles() {
	fmt.Println("\n--- Règles du jeu du pendu ---")
	fmt.Println("1. Vous devez deviner un mot en entrant une lettre à la fois.")
	fmt.Println("2. Si la lettre est dans le mot, elle est révélée.")
	fmt.Println("3. Si la lettre n'est pas dans le mot, vous perdez une vie.")
	fmt.Println("4. Vous avez un maximum de 6 erreurs possibles.")
	fmt.Println("5. Si vous devinez le mot avant d'épuiser vos vies, vous gagnez.")
	fmt.Println("6. Si vous faites 6 erreurs, vous perdez.")
	fmt.Println("7. Il existe deux niveaux de difficulté :")
	fmt.Println("    - Niveau Facile : mots de 3 à 5 lettres, avec un chrono de 1 minute 30.")
	fmt.Println("    - Niveau Difficile : mots de 6 lettres ou plus, avec un chrono de 3 minutes.")
	fmt.Println("--------------------------------\n")
}

func afficherMenu() {
	fmt.Println("=== Jeu du Pendu ===")
	fmt.Println("1. Jouer au Pendu")
	fmt.Println("2. Règles du jeu")
	fmt.Println("3. Quitter")
	fmt.Print("Choisissez une option : ")
}

func choisirNiveau() string {
	fmt.Println("Choisissez un niveau de difficulté :")
	fmt.Println("1. Facile (3-5 lettres, avec chrono de 1 minute 30)")
	fmt.Println("2. Difficile (6 lettres ou plus, avec chrono de 3 minutes)")
	fmt.Print("Votre choix : ")
	var niveau string
	fmt.Scan(&niveau)
	return niveau
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		afficherMenu()

		scanner.Scan()
		choix := scanner.Text()

		switch choix {
		case "1":
			niveau := choisirNiveau()
			jouerPendu(niveau)
		case "2":
			afficherRegles()
		case "3":
			fmt.Println("Merci d'avoir joué ! À bientôt.")
			return
		default:
			fmt.Println("Choix invalide, veuillez réessayer.")
		}
	}
}