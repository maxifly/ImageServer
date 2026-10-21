package opermanager

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"imgserver/internal/pkg/actioner"
	appmetrics "imgserver/internal/pkg/metrics"
	"imgserver/internal/pkg/timerange"

	"github.com/patrickmn/go-cache"
)

type testImageProvider struct {
	code                           string
	name                           string
	ready                          bool
	properties                     *ProviderProperties
	generateCalls                  int
	generateDirectCall             []bool
	generateByParametersCalls      int
	generateParameters             []GenerationParameters
	generateByParametersDirectCall []bool
	isReadyCalls                   int
	externalID                     string
}

func newTestImageProvider(code string, ready, archive bool) *testImageProvider {
	return &testImageProvider{
		code:       code,
		name:       code,
		ready:      ready,
		externalID: code + "_operation_id",
		properties: &ProviderProperties{
			IsArchiveImageProvider: archive,
		},
	}
}

func (provider *testImageProvider) Start() error {
	return nil
}

func (provider *testImageProvider) GetImageProviderForImageServerName() string {
	return provider.name
}

func (provider *testImageProvider) GetImageProviderCode() string {
	return provider.code
}

func (provider *testImageProvider) Generate(isDirectCall bool) (string, error) {
	provider.generateCalls++
	provider.generateDirectCall = append(provider.generateDirectCall, isDirectCall)
	return provider.externalID, nil
}

func (provider *testImageProvider) GenerateByParameters(parameters GenerationParameters, isDirectCall bool) (string, error) {
	provider.generateByParametersCalls++
	provider.generateParameters = append(provider.generateParameters, parameters)
	provider.generateByParametersDirectCall = append(provider.generateByParametersDirectCall, isDirectCall)
	return provider.externalID, nil
}

func (provider *testImageProvider) GetImageSlice(string) (bool, []byte, error) {
	return true, nil, nil
}

func (provider *testImageProvider) IsReadyForRequest() bool {
	provider.isReadyCalls++
	return provider.ready
}

func (provider *testImageProvider) SetImageParameters(*ImageParameters) error {
	return nil
}

func (provider *testImageProvider) GetProperties() *ProviderProperties {
	return provider.properties
}

func newProviderTestManager(thresholdMinutes int, sleepTimes []*SleepTime) *OperMngr {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return &OperMngr{
		pendingOperations:  cache.New(cache.NoExpiration, 0),
		completeOperations: cache.New(cache.NoExpiration, 0),
		logger:             logger,
		idMutex:            NewIdMutex(),
		actioner:           actioner.NewActioner(thresholdMinutes, time.Minute),
		sleepTimes:         sleepTimes,
		metrics:            appmetrics.NewAppMetrics(),
	}
}

func addTestImageProvider(manager *OperMngr, provider *testImageProvider) {
	var imageProvider ImageProvider = provider
	manager.AddImageProvider(&imageProvider)
}

func requirePendingOperation(t *testing.T, manager *OperMngr, id string) *Operation {
	t.Helper()

	value, found := manager.pendingOperations.Get(id)
	if !found {
		t.Fatalf("pending operation %q not found", id)
	}

	operation, ok := value.(*Operation)
	if !ok {
		t.Fatalf("pending operation %q has unexpected type %T", id, value)
	}

	return operation
}

func requireCompleteOperation(t *testing.T, manager *OperMngr, id string) *Operation {
	t.Helper()

	value, found := manager.completeOperations.Get(id)
	if !found {
		t.Fatalf("complete operation %q not found", id)
	}

	operation, ok := value.(*Operation)
	if !ok {
		t.Fatalf("complete operation %q has unexpected type %T", id, value)
	}

	return operation
}

func alwaysActiveSleepTime(blackImageMode bool) *SleepTime {
	return &SleepTime{
		TimeRange: &timerange.TimeRange{
			Start: "00:00:00",
			End:   "23:59:59",
		},
		BlackImageMode: blackImageMode,
	}
}

func TestChooseImageProviderPrefersReadyNonArchiveProvider(t *testing.T) {
	manager := newProviderTestManager(1, nil)
	external := newTestImageProvider("external", true, false)
	archive := newTestImageProvider("archive", true, true)
	addTestImageProvider(manager, archive)
	addTestImageProvider(manager, external)

	selected := manager.chooseImageProvider()

	if selected == nil || (*selected).GetImageProviderCode() != external.code {
		t.Fatalf("chooseImageProvider() selected %v, want %q", selected, external.code)
	}
}

func TestChooseImageProviderSelectsOnlyReadyNonArchiveProviders(t *testing.T) {
	manager := newProviderTestManager(1, nil)
	externalOne := newTestImageProvider("external_one", true, false)
	externalTwo := newTestImageProvider("external_two", true, false)
	notReadyExternal := newTestImageProvider("not_ready_external", false, false)
	archive := newTestImageProvider("archive", true, true)
	addTestImageProvider(manager, notReadyExternal)
	addTestImageProvider(manager, archive)
	addTestImageProvider(manager, externalOne)
	addTestImageProvider(manager, externalTwo)

	allowed := map[string]bool{
		externalOne.code: true,
		externalTwo.code: true,
	}
	for attempt := 0; attempt < 100; attempt++ {
		selected := manager.chooseImageProvider()
		if selected == nil {
			t.Fatal("chooseImageProvider() returned nil with ready external providers")
		}
		if code := (*selected).GetImageProviderCode(); !allowed[code] {
			t.Fatalf("chooseImageProvider() selected %q, want one of ready external providers", code)
		}
	}
}

func TestChooseImageProviderFallsBackToArchiveProvider(t *testing.T) {
	manager := newProviderTestManager(1, nil)
	external := newTestImageProvider("external", false, false)
	archive := newTestImageProvider("archive", true, true)
	addTestImageProvider(manager, external)
	addTestImageProvider(manager, archive)

	selected := manager.chooseImageProvider()

	if selected == nil || (*selected).GetImageProviderCode() != archive.code {
		t.Fatalf("chooseImageProvider() selected %v, want %q", selected, archive.code)
	}
}

func TestStartAutoOperationReturnsErrorWhenNoProviderIsReady(t *testing.T) {
	manager := newProviderTestManager(0, nil)
	provider := newTestImageProvider("external", false, false)
	addTestImageProvider(manager, provider)

	_, err := manager.startAutoOperation()

	if err == nil || !strings.Contains(err.Error(), "no ready image provider") {
		t.Fatalf("startAutoOperation() error = %v, want no ready provider error", err)
	}
}

func TestStartOldOperationUsesOnlyArchiveProvider(t *testing.T) {
	manager := newProviderTestManager(1, nil)
	external := newTestImageProvider("external", true, false)
	archive := newTestImageProvider("archive", true, true)
	addTestImageProvider(manager, external)
	addTestImageProvider(manager, archive)

	operationID, err := manager.StartOperation("old", "")
	if err != nil {
		t.Fatalf("StartOperation(old) error = %v", err)
	}

	operation := requirePendingOperation(t, manager, operationID)
	if operation.Provider == nil || (*operation.Provider).GetImageProviderCode() != archive.code {
		t.Fatalf("StartOperation(old) provider = %v, want %q", operation.Provider, archive.code)
	}
	if operation.Type != archive.code {
		t.Errorf("StartOperation(old) type = %q, want %q", operation.Type, archive.code)
	}
	if archive.generateCalls != 1 || external.generateCalls != 0 {
		t.Errorf("generate calls: archive=%d external=%d, want archive=1 external=0", archive.generateCalls, external.generateCalls)
	}
	if len(archive.generateDirectCall) != 1 || !archive.generateDirectCall[0] {
		t.Errorf("archive direct calls = %v, want [true]", archive.generateDirectCall)
	}
}

func TestStartOldOperationReturnsErrorWithoutArchiveProvider(t *testing.T) {
	manager := newProviderTestManager(1, nil)
	external := newTestImageProvider("external", true, false)
	addTestImageProvider(manager, external)

	_, err := manager.StartOperation("old", "")

	if err == nil || !strings.Contains(err.Error(), "no ready archive image provider") {
		t.Fatalf("StartOperation(old) error = %v, want no ready archive provider error", err)
	}
}

func TestStartAutoOperationUsesArchiveProviderDuringSleep(t *testing.T) {
	manager := newProviderTestManager(1, []*SleepTime{alwaysActiveSleepTime(false)})
	external := newTestImageProvider("external", true, false)
	archive := newTestImageProvider("archive", true, true)
	addTestImageProvider(manager, external)
	addTestImageProvider(manager, archive)

	operationID, err := manager.startAutoOperation()
	if err != nil {
		t.Fatalf("startAutoOperation() error = %v", err)
	}

	operation := requirePendingOperation(t, manager, operationID)
	if operation.Provider == nil || (*operation.Provider).GetImageProviderCode() != archive.code {
		t.Fatalf("sleep operation provider = %v, want %q", operation.Provider, archive.code)
	}
	if archive.generateCalls != 1 || external.generateCalls != 0 {
		t.Errorf("generate calls: archive=%d external=%d, want archive=1 external=0", archive.generateCalls, external.generateCalls)
	}
	if len(archive.generateDirectCall) != 1 || archive.generateDirectCall[0] {
		t.Errorf("archive direct calls = %v, want [false]", archive.generateDirectCall)
	}
	if !manager.actioner.ThresholdOut(time.Now()) {
		t.Error("archive operation unexpectedly updated the last external call time")
	}
}

func TestStartAutoOperationUsesBlackPictureModeWithoutProvider(t *testing.T) {
	manager := newProviderTestManager(1, []*SleepTime{alwaysActiveSleepTime(true)})
	external := newTestImageProvider("external", true, false)
	archive := newTestImageProvider("archive", true, true)
	addTestImageProvider(manager, external)
	addTestImageProvider(manager, archive)

	operationID, err := manager.startAutoOperation()
	if err != nil {
		t.Fatalf("startAutoOperation() error = %v", err)
	}

	operation := requireCompleteOperation(t, manager, operationID)
	if operation.Provider != nil {
		t.Errorf("black picture provider = %v, want nil", operation.Provider)
	}
	if operation.Type != BLACK_PICTURE_OPERATION_TYPE {
		t.Errorf("black picture type = %q, want %q", operation.Type, BLACK_PICTURE_OPERATION_TYPE)
	}
	if operation.FileName != BLACK_FILE_NAME {
		t.Errorf("black picture file = %q, want %q", operation.FileName, BLACK_FILE_NAME)
	}
	if archive.generateCalls != 0 || external.generateCalls != 0 {
		t.Errorf("generate calls: archive=%d external=%d, want both zero", archive.generateCalls, external.generateCalls)
	}
}

func TestStartProviderOperationUsesProviderCodeAsOperationType(t *testing.T) {
	manager := newProviderTestManager(1, nil)
	provider := newTestImageProvider("provider_code", true, false)
	var imageProvider ImageProvider = provider

	operationID, err := manager.startProviderOperation(&imageProvider, nil, false)
	if err != nil {
		t.Fatalf("startProviderOperation() error = %v", err)
	}

	operation := requirePendingOperation(t, manager, operationID)
	if operation.Type != provider.code {
		t.Errorf("operation type = %q, want %q", operation.Type, provider.code)
	}
}

func TestStartDirectOperationUsesNotReadyNonArchiveProvider(t *testing.T) {
	manager := newProviderTestManager(1, nil)
	external := newTestImageProvider("external", false, false)
	archive := newTestImageProvider("archive", true, true)
	addTestImageProvider(manager, archive)
	addTestImageProvider(manager, external)

	operationID, err := manager.StartOperation("ydart", "")
	if err != nil {
		t.Fatalf("StartOperation(ydart) error = %v", err)
	}

	operation := requirePendingOperation(t, manager, operationID)
	if operation.Provider == nil || (*operation.Provider).GetImageProviderCode() != external.code {
		t.Fatalf("StartOperation(ydart) provider = %v, want %q", operation.Provider, external.code)
	}
	if external.generateCalls != 1 || archive.generateCalls != 0 {
		t.Errorf("generate calls: external=%d archive=%d, want external=1 archive=0", external.generateCalls, archive.generateCalls)
	}
	if len(external.generateDirectCall) != 1 || !external.generateDirectCall[0] {
		t.Errorf("external direct calls = %v, want [true]", external.generateDirectCall)
	}
	if external.isReadyCalls != 0 || archive.isReadyCalls != 0 {
		t.Errorf("readiness calls: external=%d archive=%d, want both zero", external.isReadyCalls, archive.isReadyCalls)
	}
}

func TestStartDirectOperationWithPromptUsesPromptProvider(t *testing.T) {
	manager := newProviderTestManager(1, nil)
	nonPromptProvider := newTestImageProvider("non_prompt", true, false)
	promptProvider := newTestImageProvider("prompt", false, false)
	promptProvider.properties.IsCanWorkWithPrompt = true
	archive := newTestImageProvider("archive", true, true)
	archive.properties.IsCanWorkWithPrompt = true
	addTestImageProvider(manager, nonPromptProvider)
	addTestImageProvider(manager, archive)
	addTestImageProvider(manager, promptProvider)

	operationID, err := manager.StartOperation("ydart", "direct prompt")
	if err != nil {
		t.Fatalf("StartOperation(ydart, prompt) error = %v", err)
	}

	operation := requirePendingOperation(t, manager, operationID)
	if operation.Provider == nil || (*operation.Provider).GetImageProviderCode() != promptProvider.code {
		t.Fatalf("StartOperation(ydart, prompt) provider = %v, want %q", operation.Provider, promptProvider.code)
	}
	if promptProvider.generateByParametersCalls != 1 {
		t.Fatalf("prompt provider GenerateByParameters calls = %d, want 1", promptProvider.generateByParametersCalls)
	}
	if len(promptProvider.generateParameters) != 1 || promptProvider.generateParameters[0].PromptText != "direct prompt" {
		t.Errorf("prompt provider parameters = %v, want prompt text %q", promptProvider.generateParameters, "direct prompt")
	}
	if len(promptProvider.generateByParametersDirectCall) != 1 || !promptProvider.generateByParametersDirectCall[0] {
		t.Errorf("prompt provider direct calls = %v, want [true]", promptProvider.generateByParametersDirectCall)
	}
	if promptProvider.isReadyCalls != 0 || nonPromptProvider.isReadyCalls != 0 || archive.isReadyCalls != 0 {
		t.Errorf(
			"readiness calls: prompt=%d non-prompt=%d archive=%d, want all zero",
			promptProvider.isReadyCalls,
			nonPromptProvider.isReadyCalls,
			archive.isReadyCalls,
		)
	}
	if nonPromptProvider.generateCalls != 0 || nonPromptProvider.generateByParametersCalls != 0 || archive.generateCalls != 0 || archive.generateByParametersCalls != 0 {
		t.Errorf("providers without matching direct prompt capability were called")
	}
}

func TestStartDirectOperationReturnsErrorWithoutSuitableProvider(t *testing.T) {
	tests := []struct {
		name      string
		prompt    string
		providers func() []*testImageProvider
	}{
		{
			name: "empty provider list",
		},
		{
			name:   "archive provider only",
			providers: func() []*testImageProvider {
				return []*testImageProvider{newTestImageProvider("archive", true, true)}
			},
		},
		{
			name:   "no provider supports prompt",
			prompt: "direct prompt",
			providers: func() []*testImageProvider {
				return []*testImageProvider{newTestImageProvider("non_prompt", true, false)}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := newProviderTestManager(1, nil)
			if test.providers != nil {
				for _, provider := range test.providers() {
					addTestImageProvider(manager, provider)
				}
			}

			_, err := manager.StartOperation("ydart", test.prompt)

			if err == nil || !strings.Contains(err.Error(), "no suitable non-archive image provider") {
				t.Fatalf("StartOperation(ydart) error = %v, want no suitable non-archive provider error", err)
			}
			if manager.pendingOperations.ItemCount() != 0 {
				t.Errorf("pending operations = %d, want 0", manager.pendingOperations.ItemCount())
			}
		})
	}
}
