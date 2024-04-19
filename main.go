package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
)

type Message struct {
	UserID    string
	ServerID  string
	Content   string
	Timestamp string
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
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "users messages",
				Required:    true,
			},
		},
	}, // her kan neste komando være
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
	}, // her kan neste comando være
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

	msg := Message{
		UserID:    m.Author.ID,
		ServerID:  m.GuildID,
		Content:   m.Content,
		Timestamp: m.Timestamp.Format(time.RFC3339),
	}

	if err := insertMessage(msg); err != nil {
		log.Printf("Failed to insert message: %v", err)
	} else {
		log.Println("Message logged successfully")
	}
}

func insertMessage(msg Message) error {
	ctx := context.Background()
	sql := `INSERT INTO Messages (UserID, ServerID, Content, Timestamp) VALUES ($1, $2, $3, $4)`

	_, err := dbpool.Exec(ctx, sql, msg.UserID, msg.ServerID, msg.Content, msg.Timestamp)
	return err
}

/**

-- Users Table Creation
CREATE TABLE IF NOT EXISTS Users (
    UserID VARCHAR(255) PRIMARY KEY
    -- Add other user attributes here if necessary, such as username, discriminator, etc.
);

-- Servers Table Creation
CREATE TABLE IF NOT EXISTS Servers (
    ServerID VARCHAR(255) PRIMARY KEY,
    ServerName TEXT NOT NULL
);

-- Messages Table Creation
CREATE TABLE IF NOT EXISTS Messages (
    MessageID BIGSERIAL PRIMARY KEY,
    UserID VARCHAR(255) NOT NULL REFERENCES Users(UserID),
    ServerID VARCHAR(255) NOT NULL REFERENCES Servers(ServerID),
    Content TEXT NOT NULL,
    Timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for querying by UserID on the Messages Table
CREATE INDEX IF NOT EXISTS idx_messages_userid ON Messages(UserID);

-- Index for querying by ServerID on the Messages Table
CREATE INDEX IF NOT EXISTS idx_messages_serverid ON Messages(ServerID);

-- Index for querying by Timestamp on the Messages Table
CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON Messages(Timestamp);


*/
