package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	_ "time/tzdata" // embed zoneinfo so Europe/Berlin works without system tzdata

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
	"golang.org/x/term"
)

var (
	cliInput             = os.Stdin
	cliOutput  io.Writer = os.Stdout
	cliError   io.Writer = os.Stderr
	isTerminal           = fileIsTerminal
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(cliError, err)
		os.Exit(1)
	}
}

type command struct {
	name string
	run  func(configPath string, args []string) error
}

type clientCommandFunc func(*scopeskill.Client, []string) error

func clientCommand(name string, run clientCommandFunc) command {
	return command{
		name: name,
		run: func(configPath string, args []string) error {
			client, err := newClient(configPath)
			if err != nil {
				return err
			}
			return run(client, args)
		},
	}
}

var commands = []command{
	{name: "auth", run: auth},
	{name: "help", run: func(_ string, _ []string) error { return usage() }},
	clientCommand("get", get),
	clientCommand("post", post),
	clientCommand("download", download),
	clientCommand("datev", datev),
	clientCommand("bericht", bericht),
	clientCommand("teamwork", teamwork),
	clientCommand("sachkonto", sachkonto),
	clientCommand("kontakt", kontakt),
	clientCommand("debitor", debitor),
	clientCommand("kreditor", kreditor),
	clientCommand("personenkonto", personenkonto),
	clientCommand("buchhaltung", buchhaltung),
	clientCommand("dimension", dimension),
	clientCommand("textbaustein", textbaustein),
	clientCommand("statistik", statistik),
	clientCommand("zahlungsbedingung", zahlungsbedingung),
	clientCommand("steuermatrix", steuermatrix),
	clientCommand("steuersachverhalt", steuersachverhalt),
	clientCommand("eingangsrechnung", eingangsrechnung),
	clientCommand("gutschrift", gutschrift),
	clientCommand("offene-posten", offenePosten),
	clientCommand("journal", journal),
	clientCommand("kasse", kasse),
	clientCommand("reisekosten", reisekosten),
	clientCommand("buchung", buchung),
}

func lookupCommand(name string) (func(configPath string, args []string) error, bool) {
	for _, cmd := range commands {
		if cmd.name == name {
			return cmd.run, true
		}
	}
	return nil, false
}

func run(args []string) error {
	configPath, commandArgs, err := parseGlobalFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return usage()
		}
		return err
	}
	if len(commandArgs) == 0 {
		return usage()
	}

	runCmd, ok := lookupCommand(commandArgs[0])
	if !ok {
		return fmt.Errorf("unknown command: %s", commandArgs[0])
	}
	return runCmd(configPath, commandArgs[1:])
}

func newClient(configPath string) (*scopeskill.Client, error) {
	config, err := scopeskill.LoadClientConfig(configPath)
	if err != nil {
		return nil, err
	}
	return scopeskill.NewClient(config), nil
}

func auth(configPath string, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "auth subcommands: login import show secret delete")
		return nil
	}
	switch args[0] {
	case "login":
		return authLogin(configPath, args[1:])
	case "import":
		return authImport(configPath, args[1:])
	case "show":
		return authShow(configPath, args[1:])
	case "secret":
		return authSecret(configPath, args[1:])
	case "delete":
		return authDelete(configPath, args[1:])
	default:
		return fmt.Errorf("unknown auth command: %s", args[0])
	}
}

func authShow(configPath string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: sv-cli auth show")
	}
	token, source, err := effectiveRESTRefreshToken(configPath)
	if err != nil {
		return err
	}
	path := scopeskill.ResolveConfigPath(configPath)
	configFile, err := scopeskill.ReadConfigFile(path)
	if err != nil {
		return err
	}
	values := configFile.Values()
	fmt.Fprintf(cliOutput, "CUSTOMER=%s\n", values[scopeskill.ConfigKeyCustomer])
	fmt.Fprintf(cliOutput, "SKR=%s\n", values[scopeskill.ConfigKeySKR])
	fmt.Fprintf(cliOutput, "%s  source=%s\n", redactRESTRefreshToken(token), source)
	return nil
}

func authSecret(configPath string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: sv-cli auth secret")
	}
	token, source, err := effectiveRESTRefreshToken(configPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(cliOutput, "%s  source=%s\n", token, source)
	return nil
}

func authDelete(configPath string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: sv-cli auth delete")
	}
	path := scopeskill.ResolveConfigPath(configPath)
	configFile, err := scopeskill.ReadConfigFile(path)
	if err != nil {
		return err
	}
	envToken := os.Getenv(scopeskill.EnvRestRefreshToken)
	configToken := configFile.Values()[scopeskill.ConfigKeyRestRefreshToken]
	if envToken == "" && configToken == "" {
		return missingRESTRefreshTokenError()
	}
	if configToken != "" {
		if err := configFile.Delete(scopeskill.ConfigKeyRestRefreshToken); err != nil {
			return err
		}
		if err := configFile.Write(); err != nil {
			return err
		}
	}
	if envToken != "" {
		fmt.Fprintf(cliError, "warning: %s is set; deleting REST_REFRESH_TOKEN from the scopeskill config will not affect the next call\n", scopeskill.EnvRestRefreshToken)
	}
	return nil
}

func effectiveRESTRefreshToken(configPath string) (string, string, error) {
	if token := os.Getenv(scopeskill.EnvRestRefreshToken); token != "" {
		return token, "env:" + scopeskill.EnvRestRefreshToken, nil
	}
	path := scopeskill.ResolveConfigPath(configPath)
	configFile, err := scopeskill.ReadConfigFile(path)
	if err != nil {
		return "", "", err
	}
	token := configFile.Values()[scopeskill.ConfigKeyRestRefreshToken]
	if token == "" {
		return "", "", missingRESTRefreshTokenError()
	}
	return token, "config", nil
}

func missingRESTRefreshTokenError() error {
	return errors.New("missing REST refresh token; run sv-cli auth login")
}

func redactRESTRefreshToken(token string) string {
	if len(token) <= 4 {
		return "…" + token
	}
	return "…" + token[len(token)-4:]
}

func authLogin(configPath string, args []string) error {
	flags := flag.NewFlagSet("auth login", flag.ContinueOnError)
	force := flags.Bool("force", false, "overwrite an existing REST refresh token")
	skrFlag := flags.String("skr", "", "bypass the SKR probe with skr03 or skr04")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: sv-cli auth login [--force] [--skr=skr03|skr04]")
	}

	path := scopeskill.ResolveConfigPath(configPath)
	configFile, err := scopeskill.ReadConfigFile(path)
	if err != nil {
		return err
	}
	if configFile.Values()[scopeskill.ConfigKeyRestRefreshToken] != "" && !*force {
		return errors.New("scopeskill config already contains REST_REFRESH_TOKEN; rerun sv-cli auth login --force to overwrite it")
	}
	if !isTerminal(cliInput) {
		return errors.New("sv-cli auth login requires a TTY; stdin is not interactive")
	}
	if os.Getenv(scopeskill.EnvRestRefreshToken) != "" {
		fmt.Fprintf(cliError, "warning: %s is set and will shadow the REST refresh token written to the scopeskill config\n", scopeskill.EnvRestRefreshToken)
	}

	credentials, err := promptInitialCredentials()
	if err != nil {
		return err
	}
	baseConfig, err := scopeskill.LoadClientConfig(configPath)
	if err != nil {
		return err
	}
	client := scopeskill.NewClient(scopeskill.Config{BaseURL: baseConfig.BaseURL})
	token, err := client.Login(credentials)
	if err != nil {
		return err
	}
	if token.RefreshToken == "" {
		return errors.New("token response did not include refresh_token")
	}
	return writeAuthConfig(path, &configFile, baseConfig.BaseURL, credentials.Customer, token.RefreshToken, token.AccessToken, *skrFlag)
}

func authImport(configPath string, args []string) error {
	flags := flag.NewFlagSet("auth import", flag.ContinueOnError)
	force := flags.Bool("force", false, "overwrite an existing REST refresh token")
	skrFlag := flags.String("skr", "", "bypass the SKR probe with skr03 or skr04")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: sv-cli auth import [--force] [--skr=skr03|skr04]")
	}

	path := scopeskill.ResolveConfigPath(configPath)
	configFile, err := scopeskill.ReadConfigFile(path)
	if err != nil {
		return err
	}
	if configFile.Values()[scopeskill.ConfigKeyRestRefreshToken] != "" && !*force {
		return errors.New("scopeskill config already contains REST_REFRESH_TOKEN; rerun sv-cli auth import --force to overwrite it")
	}
	if !isTerminal(cliInput) {
		return errors.New("sv-cli auth import requires a TTY; stdin is not interactive")
	}
	if os.Getenv(scopeskill.EnvRestRefreshToken) != "" {
		fmt.Fprintf(cliError, "warning: %s is set and will shadow the REST refresh token written to the scopeskill config\n", scopeskill.EnvRestRefreshToken)
	}

	customer, err := promptLine("Kundennummer: ")
	if err != nil {
		return err
	}
	refreshToken, err := promptPassword("Refresh Token: ")
	if err != nil {
		return err
	}
	if customer == "" {
		return errors.New("Kundennummer ist erforderlich")
	}
	if refreshToken == "" {
		return errors.New("Refresh Token ist erforderlich")
	}

	baseConfig, err := scopeskill.LoadClientConfig(configPath)
	if err != nil {
		return err
	}
	client := scopeskill.NewClient(scopeskill.Config{
		BaseURL:          baseConfig.BaseURL,
		Customer:         customer,
		RefreshToken:     refreshToken,
		AccessTokenCache: baseConfig.AccessTokenCache,
	})
	token, err := client.RefreshToken(refreshToken)
	if err != nil {
		return fmt.Errorf("refresh token validation failed; verify Kundennummer and Refresh Token: %w", err)
	}
	return writeAuthConfig(path, &configFile, baseConfig.BaseURL, customer, refreshToken, token.AccessToken, *skrFlag)
}

func writeAuthConfig(path string, configFile *scopeskill.ConfigFile, baseURL string, customer string, refreshToken string, accessToken string, skrFlag string) error {
	if err := configFile.SetAuthLogin(customer, refreshToken); err != nil {
		return err
	}
	probeCtx := &scopeskill.ProbeContext{
		Client:  scopeskill.NewClient(scopeskill.Config{BaseURL: baseURL, AccessToken: accessToken}),
		Config:  configFile,
		Stderr:  cliError,
		Prompt:  func(message string) (string, error) { return promptLine(message) },
		SKRFlag: skrFlag,
	}
	if err := scopeskill.RunProbes(scopeskill.DefaultProbes(), probeCtx); err != nil {
		return err
	}
	if err := configFile.Write(); err != nil {
		return err
	}
	fmt.Fprintf(cliOutput, "scopeskill config written: %s\n", path)
	return nil
}

func promptInitialCredentials() (scopeskill.InitialCredentials, error) {
	customer, err := promptLine("Kundennummer: ")
	if err != nil {
		return scopeskill.InitialCredentials{}, err
	}
	username, err := promptLine("Benutzername: ")
	if err != nil {
		return scopeskill.InitialCredentials{}, err
	}
	password, err := promptPassword("Passwort: ")
	if err != nil {
		return scopeskill.InitialCredentials{}, err
	}
	organisationID, err := promptLine("Organisations-ID (optional): ")
	if err != nil {
		return scopeskill.InitialCredentials{}, err
	}
	credentials := scopeskill.InitialCredentials{
		Customer:       customer,
		Username:       username,
		Password:       password,
		OrganisationID: organisationID,
	}
	switch {
	case credentials.Customer == "":
		return scopeskill.InitialCredentials{}, errors.New("Kundennummer ist erforderlich")
	case credentials.Username == "":
		return scopeskill.InitialCredentials{}, errors.New("Benutzername ist erforderlich")
	case credentials.Password == "":
		return scopeskill.InitialCredentials{}, errors.New("Passwort ist erforderlich")
	default:
		return credentials, nil
	}
}

func promptLine(prompt string) (string, error) {
	fmt.Fprint(cliError, prompt)
	return readLine(cliInput)
}

func promptPassword(prompt string) (string, error) {
	if term.IsTerminal(int(cliInput.Fd())) {
		return promptMaskedPassword(cliInput, prompt)
	}
	return promptMaskedLine(prompt)
}

func promptMaskedPassword(input *os.File, prompt string) (string, error) {
	fileDescriptor := int(input.Fd())
	if !term.IsTerminal(fileDescriptor) {
		return "", errors.New("password input is not a terminal")
	}
	state, err := term.MakeRaw(fileDescriptor)
	if err != nil {
		return "", err
	}
	defer term.Restore(fileDescriptor, state)

	fmt.Fprint(cliError, prompt)
	var builder strings.Builder
	buffer := make([]byte, 1)
	for {
		bytesRead, err := input.Read(buffer)
		if err != nil {
			return "", err
		}
		if bytesRead == 0 {
			continue
		}
		switch value := buffer[0]; value {
		case '\r', '\n':
			fmt.Fprintln(cliError)
			return strings.TrimSpace(builder.String()), nil
		case 0x03:
			return "", errors.New("password prompt interrupted")
		case 0x04:
			return "", io.EOF
		case '\b', 0x7f:
			if builder.Len() > 0 {
				currentValue := builder.String()
				builder.Reset()
				builder.WriteString(currentValue[:len(currentValue)-1])
				fmt.Fprint(cliError, "\b \b")
			}
		default:
			if value >= ' ' {
				builder.WriteByte(value)
				fmt.Fprint(cliError, "*")
			}
		}
	}
}

func promptMaskedLine(prompt string) (string, error) {
	fmt.Fprint(cliError, prompt)
	trimmedValue, err := readLine(cliInput)
	if err != nil {
		return "", err
	}
	fmt.Fprintln(cliError, strings.Repeat("*", len([]rune(trimmedValue))))
	return trimmedValue, nil
}

func readLine(input *os.File) (string, error) {
	var builder strings.Builder
	buffer := make([]byte, 1)
	for {
		bytesRead, err := input.Read(buffer)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return strings.TrimSpace(builder.String()), nil
			}
			return "", err
		}
		if bytesRead == 0 {
			continue
		}
		switch value := buffer[0]; value {
		case '\r', '\n':
			return strings.TrimSpace(builder.String()), nil
		default:
			builder.WriteByte(value)
		}
	}
}

func get(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("get", flag.ContinueOnError)
	query := queryFlags{}
	flags.Var(&query, "query", "query parameter KEY=VALUE")
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: sv-cli get <path> [--query KEY=VALUE]")
	}
	result, err := client.JSON("GET", flags.Arg(0), nil, query)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func post(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("post", flag.ContinueOnError)
	data := flags.String("data", "", "JSON body, or @path/to/file.json")
	query := queryFlags{}
	flags.Var(&query, "query", "query parameter KEY=VALUE")
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: sv-cli post <path> --data JSON")
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

func download(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("download", flag.ContinueOnError)
	out := flags.String("out", "", "output file path")
	query := queryFlags{}
	flags.Var(&query, "query", "query parameter KEY=VALUE")
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *out == "" {
		return fmt.Errorf("usage: sv-cli download <path> --out <file>")
	}
	if err := client.Download(flags.Arg(0), *out, query); err != nil {
		return err
	}
	fmt.Fprintln(cliOutput, *out)
	return nil
}

func teamwork(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "teamwork subcommands: upload")
		return errors.New("missing teamwork subcommand")
	}
	switch args[0] {
	case "upload":
		return teamworkUpload(client, args[1:])
	default:
		return fmt.Errorf("unknown teamwork command: %s", args[0])
	}
}

func teamworkUpload(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("teamwork upload", flag.ContinueOnError)
	metadataArg := flags.String("metadata", "", "JSON metadata, or @path/to/file.json")
	collections := repeatedFlag{}
	tags := repeatedFlag{}
	flags.Var(&collections, "collection", "collection ID to add the document to")
	flags.Var(&tags, "tag", "tag to add to the document")
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: sv-cli teamwork upload <file> [--metadata JSON] [--collection ID] [--tag TAG]")
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
	fmt.Fprintln(cliOutput, string(raw))
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
	flags := flag.NewFlagSet("sv-cli", flag.ContinueOnError)
	flags.SetOutput(cliError)
	// Suppress the flag package's default usage listing; run() owns
	// usage output and prints the CLI banner on flag.ErrHelp. Parse
	// errors still propagate so invalid global flags are reported.
	flags.Usage = func() {}
	configPath := flags.String("config", "", "scopeskill config path")
	if err := flags.Parse(args); err != nil {
		return "", nil, err
	}
	return *configPath, flags.Args(), nil
}

func usage() error {
	fmt.Fprintln(cliOutput, `usage: sv-cli [--config <path>] <command> [options]

commands:
  auth              manage the configured REST refresh token
  get               run an authenticated GET request
  post              run an authenticated POST request
  download          download bytes from an authenticated endpoint
  teamwork          Teamwork-specific operations
  datev             import and export DATEV EXTF files
  bericht           retrieve and export BWA, Bilanz, and GuV reports
  sachkonto         search and inspect Sachkonten
  kontakt           search, inspect, and create Kontakte (master directory)
  debitor           search, inspect, create, and update Debitoren
  kreditor          search, inspect, create, and update Kreditoren
  personenkonto     search personal account Journal entries
  buchhaltung       inspect accounting configuration
  dimension         search dimensions and inspect entries
  textbaustein      list Textbausteine
  statistik         search and inspect Statistik accounts and postings
  zahlungsbedingung list and inspect Zahlungsbedingungen
  steuermatrix      list Steuermatrix entries
  eingangsrechnung  search, inspect, import, and repair Eingangsrechnungen
  gutschrift        search and inspect Gutschriften
  offene-posten     search, inspect, clear, rebook, and set reminder levels
  journal           search the Journal (chronological postings)
  kasse             list Kassen and create Kassenbuchungen
  buchung           inspect and manage Buchungen and their attached Belege
  reisekosten       list Reisekosten and create Nebenkosten, Übernachtung, Fahrtkosten positions`)
	return nil
}

func fileIsTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
