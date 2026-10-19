package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zachfire9/agent-harness/internal/llm"
)

type JobStatus string

const (
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobScheduled JobStatus = "scheduled"
)

type JobState struct {
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Status        JobStatus `json:"status"`
	LastRunAt     time.Time `json:"last_run_at"`
	LastSuccessAt time.Time `json:"last_success_at"`
	LastError     string    `json:"last_error"`
	NextRunAt     time.Time `json:"next_run_at"`
	OutputPath    string    `json:"output_path,omitempty"`
}

type JobsState struct {
	Jobs map[string]JobState `json:"jobs"`
}

func ReadJobs(path string) (JobsState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return JobsState{}, err
	}
	var jobs JobsState
	if err := json.Unmarshal(data, &jobs); err != nil {
		return JobsState{}, err
	}
	if jobs.Jobs == nil {
		jobs.Jobs = map[string]JobState{}
	}
	return jobs, nil
}

func WriteJobs(path string, jobs JobsState) error {
	if jobs.Jobs == nil {
		jobs.Jobs = map[string]JobState{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

type Job interface {
	Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error)
}

type Message struct {
	Job       string
	Body      string
	Timestamp time.Time
}

type Notifier interface {
	Send(ctx context.Context, message Message) error
}

type heartbeatJob struct{}

func (heartbeatJob) Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error) {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultHeartbeatJobInterval
	}
	return JobState{
		Name:          cfg.Name,
		Type:          cfg.Type,
		Status:        JobSucceeded,
		LastRunAt:     now.UTC(),
		LastSuccessAt: now.UTC(),
		LastError:     "",
		NextRunAt:     now.Add(interval).UTC(),
	}, nil
}

type localCheckinJob struct{}

type localCheckinRecord struct {
	Job       string `json:"job"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Status    string `json:"status"`
}

func (localCheckinJob) Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error) {
	outputPath := localCheckinOutputPath(cfg)
	if err := validateInstanceRelativePath(outputPath); err != nil {
		return JobState{OutputPath: outputPath}, err
	}
	record := localCheckinRecord{
		Job:       cfg.Name,
		Timestamp: now.UTC().Format(time.RFC3339),
		Message:   localCheckinMessage(cfg),
		Status:    string(JobSucceeded),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return JobState{}, err
	}
	data = append(data, '\n')
	absPath := filepath.Join(paths.Home, filepath.FromSlash(outputPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return JobState{OutputPath: outputPath}, err
	}
	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return JobState{OutputPath: outputPath}, err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return JobState{OutputPath: outputPath}, err
	}
	return JobState{
		Name:          cfg.Name,
		Type:          cfg.Type,
		Status:        JobSucceeded,
		LastRunAt:     now.UTC(),
		LastSuccessAt: now.UTC(),
		LastError:     "",
		NextRunAt:     now.Add(cfg.Interval).UTC(),
		OutputPath:    outputPath,
	}, nil
}

func localCheckinMessage(cfg JobConfig) string {
	if strings.TrimSpace(cfg.Message) == "" {
		return "agent-harness is alive"
	}
	return cfg.Message
}

func localCheckinOutputPath(cfg JobConfig) string {
	if strings.TrimSpace(cfg.OutputPath) == "" {
		return "work/checkins.jsonl"
	}
	return filepath.ToSlash(filepath.Clean(cfg.OutputPath))
}

type notifyTestJob struct{}

func (notifyTestJob) Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error) {
	return runNotifyTestWithNotifier(paths, now, cfg, notifierForJob(paths, cfg))
}

func notifierForJob(paths Paths, cfg JobConfig) Notifier {
	if cfg.Notifier.Type == "gmail" {
		return gmailNotifier{paths: paths, google: cfg.Google, config: cfg.Notifier.Gmail}
	}
	return fileOutboxNotifier{paths: paths, outboxPath: notifyTestOutboxPath(cfg)}
}

func runNotifyTestWithNotifier(paths Paths, now time.Time, cfg JobConfig, notifier Notifier) (JobState, error) {
	message := Message{Job: cfg.Name, Body: notifyTestMessage(cfg), Timestamp: now.UTC()}
	if err := notifier.Send(context.Background(), message); err != nil {
		return JobState{OutputPath: notifyTestStateOutputPath(cfg)}, err
	}
	return JobState{
		Name:          cfg.Name,
		Type:          cfg.Type,
		Status:        JobSucceeded,
		LastRunAt:     now.UTC(),
		LastSuccessAt: now.UTC(),
		LastError:     "",
		NextRunAt:     now.Add(cfg.Interval).UTC(),
		OutputPath:    notifyTestStateOutputPath(cfg),
	}, nil
}

func notifyTestMessage(cfg JobConfig) string {
	if strings.TrimSpace(cfg.Message) == "" {
		return "agent-harness notification test"
	}
	return cfg.Message
}

func notifyTestOutboxPath(cfg JobConfig) string {
	if strings.TrimSpace(cfg.OutboxPath) == "" {
		return "work/outbox.jsonl"
	}
	return filepath.ToSlash(filepath.Clean(cfg.OutboxPath))
}

func notifyTestStateOutputPath(cfg JobConfig) string {
	if cfg.Notifier.Type == "gmail" {
		return ""
	}
	return notifyTestOutboxPath(cfg)
}

type aiEmailJob struct {
	clientFactory   func(LLMConfig) (llm.ChatClient, error)
	notifierFactory func(Paths, JobConfig) Notifier
}

func (job aiEmailJob) Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error) {
	clientFactory := job.clientFactory
	if clientFactory == nil {
		clientFactory = newLLMClientForConfig
	}
	notifierFactory := job.notifierFactory
	if notifierFactory == nil {
		notifierFactory = notifierForJob
	}
	client, err := clientFactory(cfg.ResolvedLLM())
	if err != nil {
		return JobState{}, err
	}
	return runAIEmailWithClients(paths, now, cfg, notifierFactory(paths, cfg), client)
}

func runAIEmailWithClients(paths Paths, now time.Time, cfg JobConfig, notifier Notifier, client llm.ChatClient) (JobState, error) {
	llmConfig := cfg.ResolvedLLM()
	request := llm.NewChatRequest(llmConfig.Model,
		llm.Message{Role: llm.RoleSystem, Content: "Generate concise email body content for a scheduled agent-harness notification. Return only the message body."},
		llm.Message{Role: llm.RoleUser, Content: cfg.Prompt},
	)
	response, err := client.Chat(context.Background(), request)
	if err != nil {
		return JobState{}, errors.New("llm generation failed")
	}
	body := strings.TrimSpace(response.Message.Content)
	if cfg.MaxChars > 0 && len(body) > cfg.MaxChars {
		body = body[:cfg.MaxChars]
	}
	if body == "" {
		return JobState{}, errors.New("llm generation returned empty content")
	}
	message := Message{Job: cfg.Name, Body: body, Timestamp: now.UTC()}
	if err := notifier.Send(context.Background(), message); err != nil {
		return JobState{}, err
	}
	return JobState{
		Name:          cfg.Name,
		Type:          cfg.Type,
		Status:        JobSucceeded,
		LastRunAt:     now.UTC(),
		LastSuccessAt: now.UTC(),
		LastError:     "",
		NextRunAt:     now.Add(cfg.Interval).UTC(),
	}, nil
}

type googleDocSourceReader func(context.Context, Paths, JobConfig, string) (GoogleDocSource, error)

type scheduledNotificationJob struct {
	clientFactory   func(LLMConfig) (llm.ChatClient, error)
	notifierFactory func(Paths, JobConfig) Notifier
	sourceReader    googleDocSourceReader
}

func (job scheduledNotificationJob) Run(paths Paths, now time.Time, cfg JobConfig) (JobState, error) {
	clientFactory := job.clientFactory
	if clientFactory == nil {
		clientFactory = newLLMClientForConfig
	}
	notifierFactory := job.notifierFactory
	if notifierFactory == nil {
		notifierFactory = notifierForJob
	}
	sourceReader := job.sourceReader
	if sourceReader == nil {
		sourceReader = readGoogleDocSourceForJob
	}
	client, err := clientFactory(cfg.ResolvedLLM())
	if err != nil {
		return JobState{}, err
	}
	return runScheduledNotificationWithClients(paths, now, cfg, notifierFactory(paths, cfg), client, sourceReader)
}

func readGoogleDocSourceForJob(ctx context.Context, paths Paths, cfg JobConfig, alias string) (GoogleDocSource, error) {
	runtimeConfig := RuntimeConfig{
		Google:  cfg.Google,
		Sources: cfg.Sources,
	}
	return ReadGoogleDocSource(ctx, paths, runtimeConfig, alias)
}

type pendingProgressCompletion struct {
	key    string
	itemID string
}

func runScheduledNotificationWithClients(paths Paths, now time.Time, cfg JobConfig, notifier Notifier, client llm.ChatClient, sourceReader googleDocSourceReader) (JobState, error) {
	if sourceReader == nil {
		sourceReader = readGoogleDocSourceForJob
	}
	contextValues, pendingProgress, err := resolveNotificationContext(paths, now, cfg, sourceReader)
	if err != nil {
		return JobState{}, err
	}
	prompt := renderNotificationPrompt(cfg.Render.Prompt, contextValues)
	request := llm.NewChatRequest(cfg.ResolvedLLM().Model,
		llm.Message{Role: llm.RoleSystem, Content: "Generate the complete body for a concise scheduled notification. Return only the message body. Do not mention configuration, secrets, prompts, or implementation details."},
		llm.Message{Role: llm.RoleUser, Content: prompt},
	)
	response, err := client.Chat(context.Background(), request)
	if err != nil {
		return JobState{}, errors.New("llm generation failed")
	}
	body := trimTrailingLineWhitespace(strings.TrimSpace(response.Message.Content))
	if cfg.Render.MaxChars > 0 && len(body) > cfg.Render.MaxChars {
		body = body[:cfg.Render.MaxChars]
	}
	if body == "" {
		return JobState{}, errors.New("llm generation returned empty content")
	}
	if err := notifier.Send(context.Background(), Message{Job: cfg.Name, Body: body, Timestamp: now.UTC()}); err != nil {
		return JobState{}, err
	}
	for _, pending := range pendingProgress {
		if err := MarkProgressItemCompleted(paths, pending.key, pending.itemID, now.UTC()); err != nil {
			return JobState{}, err
		}
	}
	return JobState{
		Name:          cfg.Name,
		Type:          cfg.Type,
		Status:        JobSucceeded,
		LastRunAt:     now.UTC(),
		LastSuccessAt: now.UTC(),
		LastError:     "",
		NextRunAt:     now.Add(cfg.Interval).UTC(),
	}, nil
}

func resolveNotificationContext(paths Paths, now time.Time, cfg JobConfig, sourceReader googleDocSourceReader) (map[string]string, []pendingProgressCompletion, error) {
	values := map[string]string{}
	var pending []pendingProgressCompletion
	for _, item := range cfg.Context {
		switch item.Type {
		case "days_until_date":
			value, err := daysUntilDateValue(item, now)
			if err != nil {
				return nil, nil, err
			}
			values[item.ID] = value
		case "current_date":
			value, err := currentDateValue(item, now)
			if err != nil {
				return nil, nil, err
			}
			values[item.ID] = value
		case "rotating_source_item":
			source, sourceID, err := readNotificationSource(context.Background(), paths, cfg, item, sourceReader)
			if err != nil {
				return nil, nil, err
			}
			items, err := ParseProgressItemsWithHeadingPattern(item.Source.Parser, source.Text, item.Source.HeadingPattern)
			if err != nil {
				return nil, nil, err
			}
			selection, err := SelectNextProgressItem(paths, ProgressSelectionRequest{
				Key:         item.Selection.ProgressKey,
				SourceAlias: sourceID,
				SourceType:  item.Source.Type,
				Parser:      item.Source.Parser,
				Items:       items,
				Now:         now.UTC(),
			})
			if err != nil {
				return nil, nil, err
			}
			values[item.ID] = progressItemValue(selection.Item)
			pending = append(pending, pendingProgressCompletion{key: item.Selection.ProgressKey, itemID: selection.Item.ID})
		default:
			return nil, nil, fmt.Errorf("unknown notification context type %q", item.Type)
		}
	}
	return values, pending, nil
}

func progressItemValue(item ProgressItem) string {
	if len(item.Body) == 0 {
		return item.Title
	}
	lines := append([]string{item.Title}, item.Body...)
	return strings.Join(lines, "\n")
}

func readNotificationSource(ctx context.Context, paths Paths, cfg JobConfig, item NotificationContextConfig, googleReader googleDocSourceReader) (GoogleDocSource, string, error) {
	switch item.Source.Type {
	case "google_doc":
		source, err := googleReader(ctx, paths, cfg, item.Source.Alias)
		return source, "google_doc:" + item.Source.Alias, err
	case "local_file":
		path := filepath.ToSlash(filepath.Clean(item.Source.Path))
		if err := validateInstanceRelativePath(path); err != nil {
			return GoogleDocSource{}, "", err
		}
		data, err := os.ReadFile(filepath.Join(paths.Home, filepath.FromSlash(path)))
		if err != nil {
			return GoogleDocSource{}, "", err
		}
		return GoogleDocSource{Alias: path, Title: path, Text: string(data)}, "local_file:" + path, nil
	default:
		return GoogleDocSource{}, "", fmt.Errorf("unsupported source type %q", item.Source.Type)
	}
}

func currentDateValue(cfg NotificationContextConfig, now time.Time) (string, error) {
	loc := time.UTC
	if strings.TrimSpace(cfg.Timezone) != "" {
		loaded, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			return "", err
		}
		loc = loaded
	}
	t := now.In(loc)
	format := cfg.Format
	if strings.TrimSpace(format) == "" {
		format = "Monday January 2, 2006"
	}
	format = strings.ReplaceAll(format, `\n`, "\n")
	format = localizeDateFormat(format, t)
	return t.Format(format), nil
}

func localizeDateFormat(format string, t time.Time) string {
	weekday := int(t.Weekday())
	month := int(t.Month())
	day := t.Day()
	replacements := map[string]string{
		"{weekday_es}": []string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}[weekday],
		"{weekday_it}": []string{"domenica", "lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato"}[weekday],
		"{weekday_fr}": []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}[weekday],
		"{weekday_de}": []string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}[weekday],
		"{month_es}":   []string{"", "enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}[month],
		"{month_it}":   []string{"", "gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"}[month],
		"{month_fr}":   []string{"", "janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}[month],
		"{month_de}":   []string{"", "Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}[month],
		"{day_es}":     localizedNumber(day, "es"),
		"{day_it}":     localizedNumber(day, "it"),
		"{day_fr}":     localizedNumber(day, "fr"),
		"{day_de_ord}": germanOrdinal(day),
	}
	for placeholder, value := range replacements {
		format = strings.ReplaceAll(format, placeholder, value)
	}
	return format
}

func localizedNumber(n int, lang string) string {
	numbers := map[string][]string{
		"es": {"", "uno", "dos", "tres", "cuatro", "cinco", "seis", "siete", "ocho", "nueve", "diez", "once", "doce", "trece", "catorce", "quince", "dieciséis", "diecisiete", "dieciocho", "diecinueve", "veinte", "veintiuno", "veintidós", "veintitrés", "veinticuatro", "veinticinco", "veintiséis", "veintisiete", "veintiocho", "veintinueve", "treinta", "treinta y uno"},
		"it": {"", "uno", "due", "tre", "quattro", "cinque", "sei", "sette", "otto", "nove", "dieci", "undici", "dodici", "tredici", "quattordici", "quindici", "sedici", "diciassette", "diciotto", "diciannove", "venti", "ventuno", "ventidue", "ventitré", "ventiquattro", "venticinque", "ventisei", "ventisette", "ventotto", "ventinove", "trenta", "trentuno"},
		"fr": {"", "un", "deux", "trois", "quatre", "cinq", "six", "sept", "huit", "neuf", "dix", "onze", "douze", "treize", "quatorze", "quinze", "seize", "dix-sept", "dix-huit", "dix-neuf", "vingt", "vingt et un", "vingt-deux", "vingt-trois", "vingt-quatre", "vingt-cinq", "vingt-six", "vingt-sept", "vingt-huit", "vingt-neuf", "trente", "trente et un"},
	}
	if values, ok := numbers[lang]; ok && n > 0 && n < len(values) {
		return values[n]
	}
	return fmt.Sprintf("%d", n)
}

func germanOrdinal(n int) string {
	ordinals := []string{"", "erster", "zweiter", "dritter", "vierter", "fünfter", "sechster", "siebter", "achter", "neunter", "zehnter", "elfter", "zwölfter", "dreizehnter", "vierzehnter", "fünfzehnter", "sechzehnter", "siebzehnter", "achtzehnter", "neunzehnter", "zwanzigster", "einundzwanzigster", "zweiundzwanzigster", "dreiundzwanzigster", "vierundzwanzigster", "fünfundzwanzigster", "sechsundzwanzigster", "siebenundzwanzigster", "achtundzwanzigster", "neunundzwanzigster", "dreißigster", "einunddreißigster"}
	if n > 0 && n < len(ordinals) {
		return ordinals[n]
	}
	return fmt.Sprintf("%d.", n)
}

func daysUntilDateValue(cfg NotificationContextConfig, now time.Time) (string, error) {
	target, err := time.Parse("2006-01-02", cfg.Date)
	if err != nil {
		return "", err
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	days := int(target.Sub(today).Hours() / 24)
	format := cfg.OutputFormat
	if strings.TrimSpace(format) == "" {
		format = "{days} days until {label}"
	}
	value := strings.ReplaceAll(format, "{days}", commaInt(days))
	value = strings.ReplaceAll(value, "{label}", cfg.Label)
	value = strings.ReplaceAll(value, "{date}", cfg.Date)
	return value, nil
}

func renderNotificationPrompt(template string, values map[string]string) string {
	result := template
	for key, value := range values {
		result = strings.ReplaceAll(result, "{{ "+key+" }}", value)
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return result
}

func trimTrailingLineWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	return strings.Join(lines, "\n")
}

func commaInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(r))
	}
	return string(out)
}

func newLLMClientForConfig(cfg LLMConfig) (llm.ChatClient, error) {
	apiKey := strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
	if apiKey == "" {
		return nil, fmt.Errorf("llm api key env %q is not set", cfg.APIKeyEnv)
	}
	baseURL, err := llmBaseURL(cfg)
	if err != nil {
		return nil, err
	}
	return llm.NewOpenAIClient(baseURL, apiKey), nil
}

func llmBaseURL(cfg LLMConfig) (string, error) {
	if strings.TrimSpace(cfg.BaseURL) != "" {
		return cfg.BaseURL, nil
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "openrouter":
		return "https://openrouter.ai/api/v1", nil
	case "openai":
		return "https://api.openai.com/v1", nil
	case "openai_compatible":
		return "", errors.New("llm provider openai_compatible requires base_url")
	default:
		return "", fmt.Errorf("unsupported llm provider %q", cfg.Provider)
	}
}

type fileOutboxNotifier struct {
	paths      Paths
	outboxPath string
}

type fileOutboxRecord struct {
	Job       string `json:"job"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Transport string `json:"transport"`
}

func (n fileOutboxNotifier) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateInstanceRelativePath(n.outboxPath); err != nil {
		return err
	}
	record := fileOutboxRecord{
		Job:       message.Job,
		Timestamp: message.Timestamp.UTC().Format(time.RFC3339),
		Message:   message.Body,
		Transport: "file_outbox",
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	absPath := filepath.Join(n.paths.Home, filepath.FromSlash(n.outboxPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func defaultJobRegistry() map[string]Job {
	return map[string]Job{"heartbeat": heartbeatJob{}, "local_checkin": localCheckinJob{}, "notify_test": notifyTestJob{}, "ai_email": aiEmailJob{}, "scheduled_notification": scheduledNotificationJob{}}
}

func RunConfiguredJobs(paths Paths, cfg RuntimeConfig, now time.Time) error {
	cfg.attachNotifierToJobs()
	jobsState, err := readJobsStateOrEmpty(paths.JobsPath)
	if err != nil {
		return err
	}
	registry := defaultJobRegistry()
	for _, jobCfg := range cfg.Jobs {
		if !jobCfg.Enabled {
			continue
		}
		previous, hasPrevious := jobsState.Jobs[jobCfg.Name]
		if hasPrevious && previous.NextRunAt.After(now) {
			continue
		}
		if !hasPrevious && !jobCfg.Schedule.IsZero() {
			next, err := nextRunAt(jobCfg, now)
			if err != nil {
				return err
			}
			if next.After(now.UTC()) {
				jobsState.Jobs[jobCfg.Name] = scheduledJobState(jobCfg, next)
				continue
			}
		}
		if _, err := runConfiguredJobWithState(paths, jobsState, jobCfg, now, registry); err != nil {
			if writeErr := WriteJobs(paths.JobsPath, jobsState); writeErr != nil {
				return writeErr
			}
			return err
		}
	}
	return WriteJobs(paths.JobsPath, jobsState)
}

func RunConfiguredJobNow(paths Paths, cfg RuntimeConfig, name string, now time.Time) (JobState, error) {
	return runConfiguredJobNowWithRegistry(paths, cfg, name, now, defaultJobRegistry())
}

func runConfiguredJobNowWithRegistry(paths Paths, cfg RuntimeConfig, name string, now time.Time, registry map[string]Job) (JobState, error) {
	cfg.attachNotifierToJobs()
	jobCfg, ok := findJobConfig(cfg, name)
	if !ok {
		return JobState{}, fmt.Errorf("unknown job %q", name)
	}
	if !jobCfg.Enabled {
		return JobState{}, fmt.Errorf("job %q is disabled", name)
	}
	jobsState, err := readJobsStateOrEmpty(paths.JobsPath)
	if err != nil {
		return JobState{}, err
	}
	state, runErr := runConfiguredJobWithState(paths, jobsState, jobCfg, now, registry)
	if err := WriteJobs(paths.JobsPath, jobsState); err != nil {
		return JobState{}, err
	}
	return state, runErr
}

func findJobConfig(cfg RuntimeConfig, name string) (JobConfig, bool) {
	for _, jobCfg := range cfg.Jobs {
		if jobCfg.Name == name {
			return jobCfg, true
		}
	}
	return JobConfig{}, false
}

func runConfiguredJobWithState(paths Paths, jobsState JobsState, jobCfg JobConfig, now time.Time, registry map[string]Job) (JobState, error) {
	job, ok := registry[jobCfg.Type]
	if !ok {
		state := failedJobState(jobCfg, now, fmt.Errorf("unknown job type %q", jobCfg.Type))
		jobsState.Jobs[jobCfg.Name] = state
		return state, fmt.Errorf("unknown job type %q", jobCfg.Type)
	}
	state, err := job.Run(paths, now, jobCfg)
	if err != nil {
		failed := failedJobState(jobCfg, now, err)
		if state.OutputPath != "" {
			failed.OutputPath = state.OutputPath
		}
		state = failed
	} else {
		state = normalizeJobState(state, jobCfg, now)
	}
	jobsState.Jobs[jobCfg.Name] = state
	return state, err
}

func normalizeJobState(state JobState, jobCfg JobConfig, now time.Time) JobState {
	if state.Name == "" {
		state.Name = jobCfg.Name
	}
	if state.Type == "" {
		state.Type = jobCfg.Type
	}
	if state.Status == "" {
		state.Status = JobSucceeded
	}
	if state.LastRunAt.IsZero() {
		state.LastRunAt = now.UTC()
	}
	if state.Status == JobSucceeded && state.LastSuccessAt.IsZero() {
		state.LastSuccessAt = state.LastRunAt
	}
	if next, err := nextRunAt(jobCfg, now); err == nil {
		state.NextRunAt = next
	} else if state.NextRunAt.IsZero() {
		state.NextRunAt = state.LastRunAt.Add(jobCfg.Interval).UTC()
	}
	return state
}

func failedJobState(jobCfg JobConfig, now time.Time, err error) JobState {
	interval := jobCfg.Interval
	if interval <= 0 {
		interval = defaultHeartbeatJobInterval
	}
	next := now.Add(interval).UTC()
	if scheduledNext, scheduleErr := nextRunAt(jobCfg, now); scheduleErr == nil {
		next = scheduledNext
	}
	state := JobState{
		Name:      jobCfg.Name,
		Type:      jobCfg.Type,
		Status:    JobFailed,
		LastRunAt: now.UTC(),
		LastError: err.Error(),
		NextRunAt: next,
	}
	if jobCfg.Type == "local_checkin" || jobCfg.Type == "notify_test" {
		state.OutputPath = localCheckinOutputPath(jobCfg)
		if jobCfg.Type == "notify_test" {
			state.OutputPath = notifyTestStateOutputPath(jobCfg)
		}
	}
	return state
}

func scheduledJobState(jobCfg JobConfig, next time.Time) JobState {
	return JobState{
		Name:      jobCfg.Name,
		Type:      jobCfg.Type,
		Status:    JobScheduled,
		NextRunAt: next.UTC(),
	}
}

func nextRunAt(jobCfg JobConfig, now time.Time) (time.Time, error) {
	if !jobCfg.Schedule.IsZero() {
		return nextDailyRunAt(jobCfg.Schedule, now)
	}
	interval := jobCfg.Interval
	if interval <= 0 {
		interval = defaultHeartbeatJobInterval
	}
	return now.Add(interval).UTC(), nil
}

func nextDailyRunAt(schedule JobSchedule, now time.Time) (time.Time, error) {
	clock, err := time.Parse("15:04", schedule.DailyAt)
	if err != nil {
		return time.Time{}, err
	}
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	localNow := now.In(loc)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
	if !next.After(localNow) {
		next = next.AddDate(0, 0, 1)
	}
	return next.UTC(), nil
}

func readJobsStateOrEmpty(path string) (JobsState, error) {
	jobsState := JobsState{Jobs: map[string]JobState{}}
	if existing, err := ReadJobs(path); err == nil {
		jobsState = existing
	} else if !os.IsNotExist(err) {
		return JobsState{}, err
	}
	if jobsState.Jobs == nil {
		jobsState.Jobs = map[string]JobState{}
	}
	return jobsState, nil
}

func RunHeartbeatJob(paths Paths, now time.Time, interval time.Duration) (JobState, error) {
	cfg := RuntimeConfig{Jobs: []JobConfig{{
		Name:     "heartbeat",
		Type:     "heartbeat",
		Enabled:  true,
		Interval: interval,
	}}}
	if cfg.Jobs[0].Interval <= 0 {
		cfg.Jobs[0].Interval = defaultHeartbeatJobInterval
	}
	if err := RunConfiguredJobs(paths, cfg, now); err != nil {
		return JobState{}, err
	}
	jobs, err := ReadJobs(paths.JobsPath)
	if err != nil {
		return JobState{}, err
	}
	return jobs.Jobs["heartbeat"], nil
}
