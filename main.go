package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
)

type Message struct {
	UserID   int
	ServerID int
	Content  string
}

var GuildID = flag.String("guild", "", "Test guild ID. If not passed - bot registers commands globally")
var RemoveCommands = flag.Bool("rmcmd", true, "Remove all commands after shutdowning or not")
var s *discordgo.Session
var dbpool *pgxpool.Pool

func init() { flag.Parse() }

func init() {

	envErr := godotenv.Load()
	if envErr != nil {
		log.Fatalf("Error loading .env file: %v", envErr)
	}

	BotToken := os.Getenv("BOT_TOKEN")

	var err error
	s, err = discordgo.New("Bot " + BotToken)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}
}

func connectToDB() {

	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	var err error
	databaseURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", dbUser, dbPassword, dbHost, dbPort, dbName)
	dbpool, err = pgxpool.Connect(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	fmt.Println("Connected to database.")
}

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "unholy",
		Description: "Shows the unholy messages of a given user",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "word",
				Description: "word to be added",
				Required:    true,
			},
		},
	},
	{
		Name:        "unholyadd",
		Description: "Adds a word to the DB",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "word",
				Description: "The word to be added",
				Required:    true,
			},
		},
	},
	// her kan neste komando være
}

var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
	"unholy": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		user := i.ApplicationCommandData().Options[0].UserValue(s)

		response := user.ID // her skal logiken være for å sende spørring til db og der input er brukeren og response skal være det som kommer i disc

		//user.ID gir iden til brukeren (for databasen) bare user gir discord tagen.

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: response,
			},
		})
	},
	"unholyadd": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		adminUserID := os.Getenv("ADMIN_ID")
		response := "Skill issue"

		if i.Member.User.ID != adminUserID {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: response,
				},
			})
			return
		}

		word := i.ApplicationCommandData().Options[0].StringValue()

		query := `INSERT INTO words (word) VALUES ($1) ON CONFLICT (word) DO NOTHING;`
		commandTag, err := dbpool.Exec(context.Background(), query, word)

		if err != nil {
			response := fmt.Sprintf("Failed to add word: %v", err)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: response,
				},
			})
			return
		}

		if commandTag.RowsAffected() == 0 {
			response = fmt.Sprintf("Word '%s' already exists in the database.", word)
		} else {
			response = fmt.Sprintf("Word '%s' added successfully.", word)
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: response,
			},
		})
	},

	// her kan neste comando være
}

func init() {
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
}

func main() {

	connectToDB()
	defer dbpool.Close()

	s.AddHandler(messageCreate)
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})
	err := s.Open()
	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}

	log.Println("Adding commands...")
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := s.ApplicationCommandCreate(s.State.User.ID, *GuildID, v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}

	defer s.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	log.Println("Press Ctrl+C to exit")
	<-stop

	if *RemoveCommands {
		log.Println("Removing commands...")

		registeredCommands, err := s.ApplicationCommands(s.State.User.ID, *GuildID)
		if err != nil {
			log.Fatalf("Could not fetch registered commands: %v", err)
		}

		for _, v := range registeredCommands {
			err := s.ApplicationCommandDelete(s.State.User.ID, *GuildID, v.ID)
			if err != nil {
				log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
			}
		}
	}

	log.Println("Gracefully shutting down.")
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	wordsInMessage := strings.Fields(strings.ToLower(m.Content))

	wordsQuery := `SELECT wordID, word FROM words;`
	rows, err := dbpool.Query(context.Background(), wordsQuery)
	if err != nil {
		log.Printf("Error querying the datebase: %v", err)
		return
	}
	defer rows.Close()

	wordIDs := []string{}

	for rows.Next() {
		var (
			wordID string
			word   string
		)

		if err := rows.Scan(&wordID, &word); err != nil {
			log.Printf("error scanning rows: %v", err)
			return
		}

		for _, w := range wordsInMessage {
			if w == word {
				wordIDs = append(wordIDs, wordID)
			}
		}
	}

	if err := rows.Err(); err != nil {
		log.Println("error iterating rows: %w", err)
		return
	}

	if len(wordIDs) == 0 {
		return
	}

	insertQuery := `INSERT INTO messages (UserID, Message, ServerID, wordID) VALUES ($1, $2, $3, $4);`
	_, err = dbpool.Exec(context.Background(), insertQuery, m.Author.ID, m.Content, m.GuildID, wordIDs[0])
	if err != nil {
		log.Printf("Error inserting message into database: %v", err)
		return
	}

}

/**

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";


CREATE TABLE words (
    wordID UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    word VARCHAR(255) UNIQUE
);


CREATE TABLE messages (
    UserID varchar(255),
    Timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    Message TEXT,
    ServerID varchar(255),
    wordID UUID,
    FOREIGN KEY (wordID) REFERENCES words(wordID)
);

*/

/**

	// Fetch unsername form ID
	 user, err := dg.User(userID)
    if err != nil {
        fmt.Println("Failed to get user:", err)
    } else {
        fmt.Println("Username:", user.Username)
    }

    // Fetch guild name
    guild, err := dg.Guild(guildID)
    if err != nil {
        fmt.Println("Failed to get guild:", err)
    } else {
        fmt.Println("Guild Name:", guild.Name)
    }




	uuID for ID imellom ord og innlegg - defoult newid()


	auto timestamp - defoult UNIX timestamp

	 unixTimestamp := int64(1617183600) // You can replace this with any Unix timestamp

    // Convert Unix timestamp to time.Time
    tm := time.Unix(unixTimestamp, 0)

    // Format time as string "YYYY-MM-DD HH:MM:SS"
    formattedTime := tm.Format("2006-01-02 15:04:05")

    // Print formatted time
    fmt.Println("Formatted Time:", formattedTime)


*/
