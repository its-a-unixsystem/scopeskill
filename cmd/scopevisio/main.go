package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/its-a-unixsystem/scopeskill/internal/scopevisio"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	configPath, commandArgs, err := parseGlobalFlags(args)
	if err != nil {
		return err
	}
	if len(commandArgs) == 0 {
		return usage()
	}

	switch commandArgs[0] {
	case "auth":
		return auth(commandArgs[1:])
	case "help", "-h", "--help":
		return usage()
	case "get":
		client, err := newClient(configPath)
		if err != nil {
			return err
		}
		return get(client, commandArgs[1:])
	case "post":
		client, err := newClient(configPath)
		if err != nil {
			return err
		}
		return post(client, commandArgs[1:])
	case "download":
		client, err := newClient(configPath)
		if err != nil {
			return err
		}
		return download(client, commandArgs[1:])
	case "teamwork-upload":
		client, err := newClient(configPath)
		if err != nil {
			return err
		}
		return teamworkUpload(client, commandArgs[1:])
	default:
		return fmt.Errorf("unknown command: %s", commandArgs[0])
	}
}

func newClient(configPath string) (*scopevisio.Client, error) {
	config, err := scopevisio.LoadClientConfig(configPath)
	if err != nil {
		return nil, err
	}
	return scopevisio.NewClient(config), nil
}

func auth(args []string) error {
	if len(args) == 0 {
		fmt.Println("usage: scopevisio auth <command>")
		return nil
	}
	return fmt.Errorf("unknown auth command: %s", args[0])
}

func get(client *scopevisio.Client, args []string) error {
	flags := flag.NewFlagSet("get", flag.ContinueOnError)
	query := queryFlags{}
	flags.Var(&query, "query", "query parameter KEY=VALUE")
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: scopevisio get <path> [--query KEY=VALUE]")
	}
	result, err := client.JSON("GET", flags.Arg(0), nil, query)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func post(client *scopevisio.Client, args []string) error {
	flags := flag.NewFlagSet("post", flag.ContinueOnError)
	data := flags.String("data", "", "JSON body, or @path/to/file.json")
	query := queryFlags{}
	flags.Var(&query, "query", "query parameter KEY=VALUE")
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: scopevisio post <path> --data JSON")
	}
	body, err := loadJSON(*data)
	if err != nil {
		return err
	}
	result, err := client.JSON("POST", flags.Arg(0), body, query)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func download(client *scopevisio.Client, args []string) error {
	flags := flag.NewFlagSet("download", flag.ContinueOnError)
	out := flags.String("out", "", "output file path")
	query := queryFlags{}
	flags.Var(&query, "query", "query parameter KEY=VALUE")
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *out == "" {
		return fmt.Errorf("usage: scopevisio download <path> --out <file>")
	}
	if err := client.Download(flags.Arg(0), *out, query); err != nil {
		return err
	}
	fmt.Println(*out)
	return nil
}

func teamworkUpload(client *scopevisio.Client, args []string) error {
	flags := flag.NewFlagSet("teamwork-upload", flag.ContinueOnError)
	metadataArg := flags.String("metadata", "", "JSON metadata, or @path/to/file.json")
	collections := repeatedFlag{}
	tags := repeatedFlag{}
	flags.Var(&collections, "collection", "collection ID to add the document to")
	flags.Var(&tags, "tag", "tag to add to the document")
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: scopevisio teamwork-upload <file> [--metadata JSON] [--collection ID] [--tag TAG]")
	}
	metadata, err := loadJSONObject(*metadataArg)
	if err != nil {
		return err
	}
	if len(collections) > 0 || len(tags) > 0 {
		meta := ensureMap(metadata, "metadata")
		actions := ensureMap(meta, "actions")
		if len(collections) > 0 {
			actions["add-to-collection"] = []string(collections)
		}
		if len(tags) > 0 {
			actions["add-tag"] = []string(tags)
		}
	}
	result, err := client.UploadTeamworkDocument(flags.Arg(0), metadata)
	if err != nil {
		return err
	}
	return printJSON(result)
}

type queryFlags map[string]string

func (q queryFlags) String() string {
	raw, _ := json.Marshal(map[string]string(q))
	return string(raw)
}

func (q queryFlags) Set(value string) error {
	key, val, ok := strings.Cut(value, "=")
	if !ok {
		return fmt.Errorf("invalid query parameter: %s", value)
	}
	q[key] = val
	return nil
}

type repeatedFlag []string

func (r *repeatedFlag) String() string {
	return strings.Join(*r, ",")
}

func (r *repeatedFlag) Set(value string) error {
	*r = append(*r, value)
	return nil
}

func loadJSON(value string) (any, error) {
	if value == "" {
		return nil, nil
	}
	raw, err := loadRaw(value)
	if err != nil {
		return nil, err
	}
	var result any
	return result, json.Unmarshal(raw, &result)
}

func loadJSONObject(value string) (map[string]any, error) {
	if value == "" {
		return map[string]any{}, nil
	}
	raw, err := loadRaw(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, json.Unmarshal(raw, &result)
}

func loadRaw(value string) ([]byte, error) {
	if strings.HasPrefix(value, "@") {
		return os.ReadFile(strings.TrimPrefix(value, "@"))
	}
	return []byte(value), nil
}

func ensureMap(parent map[string]any, key string) map[string]any {
	if child, ok := parent[key].(map[string]any); ok {
		return child
	}
	child := map[string]any{}
	parent[key] = child
	return child
}

func printJSON(value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}

func normalizeFlagArgs(args []string) []string {
	var flags []string
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if strings.Contains(arg, "=") {
				continue
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func parseGlobalFlags(args []string) (string, []string, error) {
	flags := flag.NewFlagSet("scopevisio", flag.ContinueOnError)
	configPath := flags.String("config", "", "Scopevisio config path")
	if err := flags.Parse(args); err != nil {
		return "", nil, err
	}
	return *configPath, flags.Args(), nil
}

func usage() error {
	fmt.Println(`usage: scopevisio [--config <path>] <command> [options]

commands:
  auth              manage the configured REST refresh token
  get               run an authenticated GET request
  post              run an authenticated POST request
  download          download bytes from an authenticated endpoint
  teamwork-upload   upload a document through Teamworkbridge`)
	return nil
}
