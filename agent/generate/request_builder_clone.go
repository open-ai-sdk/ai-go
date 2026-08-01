package generate

func cloneGenerateTextRequest(request GenerateTextRequest) GenerateTextRequest {
	request.Messages = append([]Message(nil), request.Messages...)
	request.Settings.StopSequences = append(
		[]string(nil),
		request.Settings.StopSequences...,
	)
	request.ProviderOptions = cloneMap(request.ProviderOptions)
	if request.ActiveTools != nil {
		request.ActiveTools = append([]string{}, request.ActiveTools...)
	}
	request.ToolsContext = cloneMap(request.ToolsContext)
	request.RuntimeContext = cloneMap(request.RuntimeContext)
	request.ToolApproval = cloneMap(request.ToolApproval)
	request.ToolApprovalKey = append([]byte(nil), request.ToolApprovalKey...)
	request.Middlewares = append(
		[]LanguageModelMiddleware(nil),
		request.Middlewares...,
	)
	return request
}

func withProviderOption(options map[string]any, provider string, option any) map[string]any {
	options = cloneMap(options)
	if options == nil {
		options = make(map[string]any)
	}
	options[provider] = option
	return options
}

func cloneMap[M ~map[K]V, K comparable, V any](values M) M {
	if values == nil {
		return nil
	}
	cloned := make(M, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
