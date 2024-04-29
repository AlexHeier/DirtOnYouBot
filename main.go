package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
)

type Message struct {
	UserID   int
	ServerID int
	Content  string
}

type userScore struct {
	UserID       string
	MessageCount int
	Username     string
}

var GuildID = flag.String("guild", "", "Test guild ID. If not passed - bot registers commands globally")
var RemoveCommands = flag.Bool("rmcmd", true, "Remove all commands after shutdowning or not")
var s *discordgo.Session
var dbpool *pgxpool.Pool
var wordMap map[string]string

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

	loadWordMap()
}

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "unholy",
		Description: "Shows the unholy messages of a given user",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "Select the user you want to lookup",
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
	{
		Name:        "unholyremove",
		Description: "removees a word to the database",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "word",
				Description: "The word to be removed",
				Required:    true,
			},
		},
	},
	{
		Name:        "scoreboard",
		Description: "shows who has the most messages added to the database",
	},
	{
		Name:        "words",
		Description: "shows all the words in the database",
	},
	{
		Name:        "commonwords",
		Description: "shows which words has been used to most",
	},
	{
		Name:        "deleteallmessages",
		Description: "dropes message table and restarts backtracking",
	},
	{
		Name:        "help",
		Description: "gives a small guide on how to use the bot",
	},
	// her kan neste komando være
}

var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
	"unholy": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		if err := acknowledgeInteraction(s, i); err != nil {
			return
		}

		user := i.ApplicationCommandData().Options[0].UserValue(s)
		var response string

		serverID := i.GuildID
		mainServer := os.Getenv("MAIN_SERVER")

		var query string
		if serverID != mainServer {
			query = `SELECT serverID, userID, message, timestamp FROM messages WHERE userID = $1 AND serverID = $2 ORDER BY timestamp ASC`
		} else {
			query = `SELECT serverID, userID, message, timestamp FROM messages WHERE userID = $1 ORDER BY timestamp ASC`
		}

		var err error
		var rows pgx.Rows
		if serverID != mainServer {
			rows, err = dbpool.Query(context.Background(), query, user.ID, serverID)
		} else {
			rows, err = dbpool.Query(context.Background(), query, user.ID)
		}

		if err != nil {
			response = fmt.Sprintf("Failed to query database: %v", err)
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}
		defer rows.Close()

		var messages []string
		for rows.Next() {
			var serverID, userID, message string
			var timestamp time.Time
			if err := rows.Scan(&serverID, &userID, &message, &timestamp); err != nil {
				log.Printf("Error scanning row: %v", err)
				continue
			}

			guild, err := s.Guild(serverID)
			if err != nil {
				log.Printf("Error fetching guild: %v", err)
				continue
			}

			messageStr := fmt.Sprintf("%s: %s: **%s**: %s", guild.Name, user.Username, message, timestamp.Format("2006-01-02 15:04:05"))
			messages = append(messages, messageStr)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Error processing results: %v", err)
			response = fmt.Sprintf("Error processing results: %v", err)
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		response = strings.Join(messages, "\n")
		if response == "" {
			response = fmt.Sprintf("%v is a boring bitch. Gaslight them to join the list ;D", user.Username)
		}

		if err := sendResponse(s, i, response); err != nil {
			log.Printf("Error sending detailed response: %v", err)
		}
	},

	"unholyadd": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		if err := acknowledgeInteraction(s, i); err != nil {
			return
		}
		response := "Skill issue"
		adminUserID := os.Getenv("ADMIN_ID")

		if i.Member.User.ID != adminUserID {
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		word := i.ApplicationCommandData().Options[0].StringValue()

		query := `INSERT INTO words (word) VALUES ($1) ON CONFLICT (word) DO NOTHING;`
		commandTag, err := dbpool.Exec(context.Background(), query, word)

		if err != nil {
			response := fmt.Sprintf("Failed to add word: %v", err)
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		if commandTag.RowsAffected() == 0 {
			response = fmt.Sprintf("Word '%s' already exists in the database.", word)
		} else {
			response = fmt.Sprintf("Word '%s' added successfully.", word)
		}

		loadWordMap()

		if err := sendResponse(s, i, response); err != nil {
			log.Printf("Error sending detailed response: %v", err)
		}
	},
	"unholyremove": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		if err := acknowledgeInteraction(s, i); err != nil {
			return
		}
		response := "Skill issue"
		adminUserID := os.Getenv("ADMIN_ID")

		if i.Member.User.ID != adminUserID {
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		word := i.ApplicationCommandData().Options[0].StringValue()

		query := `DELETE FROM words WHERE word = $1;`
		commandTag, err := dbpool.Exec(context.Background(), query, word)

		if err != nil {
			response := fmt.Sprintf("Failed to remove word: %v", err)
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		if err != nil {
			response := fmt.Sprintf("Failed to remove word: %v", err)
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		if commandTag.RowsAffected() == 0 {
			response = fmt.Sprintf("Word '%s' does not exists in the database.", word)
		} else {
			response = fmt.Sprintf("Word '%s' removed successfully.", word)
		}

		loadWordMap()

		if err := sendResponse(s, i, response); err != nil {
			log.Printf("Error sending detailed response: %v", err)
		}
	},
	"scoreboard": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		if err := acknowledgeInteraction(s, i); err != nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		serverID := i.GuildID
		mainServer := os.Getenv("MAIN_SERVER")

		var query string
		if serverID != mainServer {
			query = `SELECT UserID, COUNT(*) AS message_count FROM messages WHERE serverID = $1 GROUP BY UserID ORDER BY message_count DESC;`
		} else {
			query = `SELECT UserID, COUNT(*) AS message_count FROM messages GROUP BY UserID ORDER BY message_count DESC;`
		}

		var err error
		var rows pgx.Rows
		if serverID != mainServer {
			rows, err = dbpool.Query(ctx, query, serverID)
		} else {
			rows, err = dbpool.Query(ctx, query)
		}

		if err != nil {
			log.Printf("Error executing scoreboard query: %v", err)
			if err := sendResponse(s, i, "failed to fetch scoreboard"); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		if !rows.Next() {
			if err := sendResponse(s, i, "No words has been recorded yet! be the first :D"); err != nil {
				log.Printf("Error sending no data response: %v", err)
			}
			return
		}

		defer rows.Close()

		var scores []userScore

		for rows.Next() {
			var us userScore
			if err := rows.Scan(&us.UserID, &us.MessageCount); err != nil {
				log.Printf("Error scanning scoreboard row: %v", err)
				continue
			}
			scores = append(scores, us)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Error processing scoreboard results: %v", err)
			if err := sendResponse(s, i, "Error processing scoreboard"); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		// Fetch usernames for each user ID
		for index, score := range scores {
			user, err := s.User(score.UserID)
			if err != nil {
				log.Printf("Error fetching user %s: %v", score.UserID, err)
				scores[index].Username = "Unknown User"
			} else {
				scores[index].Username = user.Username
			}
		}

		embed := &discordgo.MessageEmbed{
			Title:       "Scoreboard",
			Description: "Top message counts:",
			Color:       0x00ff00, // Green color
			Fields:      make([]*discordgo.MessageEmbedField, 0),
			Timestamp:   time.Now().Format(time.RFC3339),
		}

		for i, score := range scores {
			if i >= 3 {
				break
			}
			icon := ""
			switch i {
			case 0:
				icon = "🥇"
			case 1:
				icon = "🥈"
			case 2:
				icon = "🥉"
			}
			field := &discordgo.MessageEmbedField{
				Name:   fmt.Sprintf("%s %s", icon, score.Username),
				Value:  fmt.Sprintf("%d entries", score.MessageCount),
				Inline: false,
			}
			embed.Fields = append(embed.Fields, field)
		}

		if len(scores) > 3 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "More results",
				Value:  "-------------------------",
				Inline: false,
			})
			for _, score := range scores[3:] {
				field := &discordgo.MessageEmbedField{
					Name:   score.Username,
					Value:  fmt.Sprintf("%d entries", score.MessageCount),
					Inline: false,
				}
				embed.Fields = append(embed.Fields, field)
			}
		}

		if err := sendEmbedResponse(s, i, embed); err != nil {
			log.Printf("Error sending detailed response: %v", err)
		}
	},

	"words": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		if err := acknowledgeInteraction(s, i); err != nil {
			return // Stop if we can't even acknowledge the interaction
		}

		var response string

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rows, err := dbpool.Query(ctx, "SELECT word FROM words")
		if err != nil {
			log.Printf("Error executing query: %v", err)
			response := "Error fetching words."
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}
		defer rows.Close()

		var words []string
		for rows.Next() {
			var word string
			if err := rows.Scan(&word); err != nil {
				log.Printf("Error scanning row: %v", err)
				continue
			}
			words = append(words, word)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Error during row iteration: %v", err)
			response := "Error processing words."
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		if len(words) == 0 {
			response = "No words found."
		} else {
			response = "Words: " + strings.Join(words, ", ")
		}
		if err := sendResponse(s, i, response); err != nil {
			log.Printf("Error sending detailed response: %v", err)
		}
	},
	"commonwords": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		if err := acknowledgeInteraction(s, i); err != nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		serverID := i.GuildID
		mainServer := os.Getenv("MAIN_SERVER")

		var query string
		if serverID != mainServer {
			query = `SELECT w.word, COUNT(*) AS usage_count FROM words w JOIN (SELECT UNNEST(wordID) AS wordID FROM messages WHERE serverID = $1) m ON w.wordID = m.wordID GROUP BY w.word ORDER BY usage_count DESC;`
		} else {
			query = `SELECT w.word, COUNT(*) AS usage_count FROM words w JOIN (SELECT UNNEST(wordID) AS wordID FROM messages) m ON w.wordID = m.wordID GROUP BY w.word ORDER BY usage_count DESC;`
		}

		var err error
		var rows pgx.Rows
		if serverID != mainServer {
			rows, err = dbpool.Query(ctx, query, serverID)
		} else {
			rows, err = dbpool.Query(ctx, query)
		}

		if err != nil {
			log.Printf("Error executing commonwords query: %v", err)
			if err := sendResponse(s, i, "Failed to fetch common words"); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}
		defer rows.Close()

		var results []string
		for rows.Next() {
			var word string
			var count int
			if err := rows.Scan(&word, &count); err != nil {
				log.Printf("Error scanning row: %v", err)
				continue
			}
			results = append(results, fmt.Sprintf("%s: %d", word, count))
		}

		if err := rows.Err(); err != nil {
			log.Printf("Error processing common words results: %v", err)
			if err := sendResponse(s, i, "Error processing common word response"); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		if len(results) == 0 {
			if err := sendResponse(s, i, "No words has been recorded yet! be the first :D"); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
		} else {
			response := "Common Words Usage:\n" + strings.Join(results, "\n")
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
		}
	},
	"deleteallmessages": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if err := acknowledgeInteraction(s, i); err != nil {
			return
		}

		response := "Skill issue"
		adminUserID := os.Getenv("ADMIN_ID")

		if i.Member.User.ID != adminUserID {
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		_, err := dbpool.Exec(context.Background(), `DELETE FROM messages;`)

		if err != nil {
			response := fmt.Sprintf("Failed to remove word: %v", err)
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		_, err = dbpool.Exec(context.Background(), `DELETE FROM servers;`)

		if err != nil {
			response := fmt.Sprintf("Failed to remove servers: %v", err)
			if err := sendResponse(s, i, response); err != nil {
				log.Printf("Error sending detailed response: %v", err)
			}
			return
		}

		response = "All data from messages has been deleted !!!"

		if err := sendResponse(s, i, response); err != nil {
			log.Printf("Error sending detailed response: %v", err)
		}
	},
	"help": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if err := acknowledgeInteraction(s, i); err != nil {
			return
		}

		response := `
		# Welcome to **DirtOnYou**!
		 
		Ｃｏｍｍａｎｄｓ:
		> **/unholy <user>**: This command will send all the _flagged_ messages of the given user, including information about when and where each message was sent.
		
		> **/scoreboard**: This command displays a scoreboard with the amount of _flagged_ messages all users have.
		
		> **/words**: This command will list all the _flagged_ words the bot monitors.
		
		> **/commonwords**: This command will show all words that have been used and the frequency of their usage.
		
		> **/help**: Responds with _this_ message.
		 
		Ａｄｍｉｎ Ｃｏｍｍａｎｄｓ:
		> **/unholyadd <word>**: Adds a word to be _flagged_. Note: this will only affect new messages. To apply changes to old messages, use: _/deleteallmessages_.
		
		> **/unholyremove <word>**: Removes a word from the database and stops monitoring it. Previous messages logged with this word will not be deleted. To apply changes to old messages, use: _/deleteallmessages_.
		
		> **/deleteallmessages**: Deletes all messages in the database and starts backtracking the server. Note: this process is time-consuming due to Discord's limits, estimated at 5,600 messages per minute.
		`

		if err := sendResponse(s, i, response); err != nil {
			log.Printf("Error sending detailed response: %v", err)
		}
	},

	// her kan neste commando være
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
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	serverExists, err := checkServerExists(m.GuildID)
	if err != nil {
		log.Printf("Error checking server existence: %v", err)
		return
	}
	if !serverExists {
		go processAllMessages(s, m.GuildID)
	} else {
		processMessage(s, m.Message, m.GuildID)
	}
}

func processMessage(s *discordgo.Session, msg *discordgo.Message, guildID string) {
	if msg.Author.ID == s.State.User.ID {
		return
	}

	if wordMap == nil {
		return
	}

	wordsInMessage := strings.Fields(strings.ToLower(msg.Content))
	var wordIDs []string
	for _, word := range wordsInMessage {
		for key, id := range wordMap {
			if strings.Contains(word, key) {
				wordIDs = append(wordIDs, id)
			}
		}
	}

	if len(wordIDs) == 0 {
		return
	}

	insertMessageIntoDB(msg, guildID, wordIDs)
}

func loadWordMap() {
	newWordMap := make(map[string]string)
	wordsQuery := `SELECT wordID, word FROM words;`
	rows, err := dbpool.Query(context.Background(), wordsQuery)
	if err != nil {
		log.Printf("Error querying the database: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var wordID, word string
		if err := rows.Scan(&wordID, &word); err != nil {
			log.Printf("Error scanning rows: %v", err)
			return
		}
		newWordMap[word] = wordID
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating rows: %v", err)
		return
	}

	wordMap = newWordMap
}

func insertMessageIntoDB(msg *discordgo.Message, guildID string, wordIDs []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		log.Printf("Error starting transaction: %v", err)
		return
	}
	defer tx.Rollback(ctx)

	msg.Content = strings.ReplaceAll(msg.Content, "@", "@\u200B")

	insertQuery := `INSERT INTO messages (UserID, Message, ServerID, wordID, timestamp) VALUES ($1, $2, $3, $4, $5);`
	_, err = tx.Exec(ctx, insertQuery, msg.Author.ID, msg.Content, guildID, wordIDs, msg.Timestamp)
	if err != nil {
		log.Printf("Error inserting message into database: %v", err)
		return
	}

	if err = tx.Commit(ctx); err != nil {
		log.Printf("Error committing transaction: %v", err)
	}
}

func checkServerExists(guildID string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM servers WHERE serverid=$1);`
	err := dbpool.QueryRow(context.Background(), query, guildID).Scan(&exists)

	if !exists {
		insertQuery := `INSERT INTO servers (serverid) VALUES ($1);`
		_, err = dbpool.Exec(context.Background(), insertQuery, guildID)
		if err != nil {
			return false, fmt.Errorf("error inserting new server: %v", err)
		}
	}

	return exists, err
}

func processAllMessages(s *discordgo.Session, guildID string) {
	m := ""
	totalMessageAmount := 0
	timeStart := time.Now()

	guild, err := s.Guild(guildID)
	if err != nil {
		return
	}
	guildChannels, err := s.GuildChannels(guildID)
	if err != nil {
		return
	}

	m = fmt.Sprintf("I've started backtracking **%v**, i found **%v** channels", guild.Name, len(guildChannels))
	sendAdminDM(m)

	for _, channel := range guildChannels {
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}

		m = fmt.Sprintf("Started on **%v** in **%v**", channel.Name, guild.Name)
		sendAdminDM(m)

		lastID := ""
		var allMessages int = 0

		for {
			messages, err := s.ChannelMessages(channel.ID, 100, lastID, "", "")
			if err != nil {
				log.Printf("Error fetching messages from channel %v: %v", channel.Name, err)
				break
			}
			if len(messages) == 0 {
				break
			}

			for _, msg := range messages {
				processMessage(s, msg, guildID)
			}

			allMessages += len(messages)

			lastID = messages[len(messages)-1].ID

			if len(messages) < 100 {
				break
			}
		}

		m = fmt.Sprintf("Done with **%v** in **%v** found **%v** messages.", channel.Name, guild.Name, allMessages)
		sendAdminDM(m)

		totalMessageAmount = totalMessageAmount + allMessages
	}

	m = fmt.Sprintf("Server **%v** has been backtracked. It took %v. Found a total of **%v** messages", guild.Name, time.Since(timeStart), totalMessageAmount)
	sendAdminDM(m)
}

func sendResponse(s *discordgo.Session, i *discordgo.InteractionCreate, response string) error {
	const maxMessageLength = 2000

	if len(response) <= maxMessageLength {
		_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: response,
		})
		if err != nil {
			log.Printf("Error sending follow-up message: %v", err)
		}
		return err
	}

	for k := 0; k < len(response); k += maxMessageLength {
		end := k + maxMessageLength
		if end > len(response) {
			end = len(response)
		}

		chunk := response[k:end]

		_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: chunk,
		})
		if err != nil {
			log.Printf("Error sending follow-up message: %v", err)
			return err
		}
	}

	return nil
}

func sendEmbedResponse(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) error {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		log.Printf("Error sending follow-up embed message: %v", err)
	}
	return err
}

func acknowledgeInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("Error acknowledging interaction: %v", err)
	}
	return err
}

func sendAdminDM(message string) {
	adminUserID := os.Getenv("ADMIN_ID")

	channel, err := s.UserChannelCreate(adminUserID)
	if err != nil {
		log.Printf("Error creating DM channel: %v", err)
		return
	}

	_, err = s.ChannelMessageSend(channel.ID, message)
	if err != nil {
		log.Printf("Error sending DM: %v", err)
		return
	}
}

/**

Database Schema PostgreSQL:

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS public.messages (
    userid character varying(255) COLLATE pg_catalog."default",
    timestamp timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    message text COLLATE pg_catalog."default",
    serverid character varying(255) COLLATE pg_catalog."default",
    wordid uuid[]
);


CREATE TABLE IF NOT EXISTS public.words (
    wordid uuid NOT NULL DEFAULT uuid_generate_v4(),
    word character varying(255) COLLATE pg_catalog."default",
    CONSTRAINT words_pkey PRIMARY KEY (wordid),
    CONSTRAINT words_word_key UNIQUE (word)
);

CREATE TABLE IF NOT EXISTS public.servers (
    serverid varchar(255)
)

ALTER TABLE IF EXISTS public.messages OWNER TO postgres;
ALTER TABLE IF EXISTS public.words OWNER TO postgres;
*/
