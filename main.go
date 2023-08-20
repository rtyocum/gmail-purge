package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	codeChan := make(chan string)
	serverExit := make(chan bool)

	handler := http.NewServeMux()

	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		codeChan <- r.URL.Query().Get("code")
		w.Write([]byte("You may now close this window."))
	})

	srv := &http.Server{Addr: ":8080", Handler: handler}
	go func() {
		srv.ListenAndServe()
		serverExit <- true
	}()

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	openBrowser(authURL)

	// get code
	code := <-codeChan

	tok, err := config.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	// shutdown server
	srv.Close()
	<-serverExit

	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func openBrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}

}

func getMessagesRecursive(category string, srv *gmail.Service, pageToken string) []*gmail.Message {
	var messageList []*gmail.Message
	messages, err := srv.Users.Messages.List("me").Q("category:" + category).MaxResults(500).PageToken(pageToken).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve messages: %v", err)
	}

	messageList = append(messageList, messages.Messages...)

	if messages.NextPageToken != "" {
		messageList = append(messageList, getMessagesRecursive(category, srv, messages.NextPageToken)...)
	}

	return messageList
}

func getMessages(category string, srv *gmail.Service) []*gmail.Message {
	var messageList []*gmail.Message
	messages, err := srv.Users.Messages.List("me").Q("category:" + category).MaxResults(500).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve messages: %v", err)
	}

	messageList = append(messageList, messages.Messages...)

	if messages.NextPageToken != "" {
		messageList = append(messageList, getMessagesRecursive(category, srv, messages.NextPageToken)...)
	}

	return messageList
}

func splitMessages(messages []string, n int) [][]string {
	var chunks [][]string
	for i := 0; i < len(messages); i += n {
		end := i + n
		if end > len(messages) {
			end = len(messages)
		}
		chunks = append(chunks, messages[i:end])
	}
	return chunks
}

func main() {
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, gmail.MailGoogleComScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Gmail client: %v", err)
	}

	user := "me"
	// Get all categories

	fmt.Println("Categories:")
	fmt.Println("1 - Primary")
	fmt.Println("2 - Social")
	fmt.Println("3 - Promotions")
	fmt.Println("4 - Updates")
	fmt.Println("5 - Forums")

	// Get input from user:
	var purgeCategory string
	fmt.Print("Enter category number to purge: ")
	fmt.Scanln(&purgeCategory)

	switch purgeCategory {
	case "1":
		purgeCategory = "primary"
	case "2":
		purgeCategory = "social"
	case "3":
		purgeCategory = "promotions"
	case "4":
		purgeCategory = "updates"
	case "5":
		purgeCategory = "forums"
	default:
		fmt.Println("Invalid category number")
		os.Exit(1)
	}

	var confirm string
	fmt.Printf("Are you sure you want to purge all messages in %s? (Y/n): ", purgeCategory)
	fmt.Scanln(&confirm)

	if confirm != "Y" {
		fmt.Println("Exiting...")
		os.Exit(1)
	}

	// Get all messages in category

	messages := getMessages(purgeCategory, srv)

	var messageIds []string
	for _, m := range messages {
		messageIds = append(messageIds, m.Id)
	}

	var confirmDelete string
	fmt.Printf("Are you sure you want to delete %d messages? (Y/n): ", len(messageIds))
	fmt.Scanln(&confirmDelete)

	if confirmDelete != "Y" {
		fmt.Println("Exiting...")
		os.Exit(1)
	}

	// Split messages into chunks of 1000
	chunks := splitMessages(messageIds, 1000)

	// Delete messages in chunks

	for _, c := range chunks {
		batchDelete := gmail.BatchDeleteMessagesRequest{Ids: c}
		err = srv.Users.Messages.BatchDelete(user, &batchDelete).Do()
		if err != nil {
			log.Fatalf("Unable to delete messages: %v", err)
		}
	}

	fmt.Printf("Deleted %d messages\n", len(messageIds))
}
