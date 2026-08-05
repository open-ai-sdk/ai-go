package ainode

type ToolChunkOpts struct {
	ProviderExecuted *bool
	ProviderMetadata map[string]any
	ToolMetadata     map[string]any
	Dynamic          *bool
	Title            string
	Preliminary      *bool
}

func applyToolOpts(fields map[string]any, options *ToolChunkOpts) map[string]any {
	if options == nil {
		return fields
	}
	if options.ProviderExecuted != nil {
		fields["providerExecuted"] = *options.ProviderExecuted
	}
	fields = withProviderMetadata(fields, options.ProviderMetadata)
	if options.ToolMetadata != nil {
		fields["toolMetadata"] = options.ToolMetadata
	}
	if options.Dynamic != nil {
		fields["dynamic"] = *options.Dynamic
	}
	if options.Title != "" {
		fields["title"] = options.Title
	}
	if options.Preliminary != nil {
		fields["preliminary"] = *options.Preliminary
	}
	return fields
}

func (writer *Writer) WriteToolInputError(id, name string, input any, errorText string, options *ToolChunkOpts) error {
	fields := applyToolOpts(map[string]any{
		"toolCallId": id, "toolName": name, "input": input, "errorText": errorText,
	}, options)
	return WriteSSE(writer.w, Chunk{Type: ChunkToolInputError, Fields: fields})
}

func (writer *Writer) WriteToolOutputError(id, errorText string, options *ToolChunkOpts) error {
	fields := applyToolOpts(map[string]any{"toolCallId": id, "errorText": errorText}, options)
	return WriteSSE(writer.w, Chunk{Type: ChunkToolOutputError, Fields: fields})
}

func (writer *Writer) WriteToolOutputDenied(id string, options *ToolChunkOpts) error {
	fields := applyToolOpts(map[string]any{"toolCallId": id}, options)
	return WriteSSE(writer.w, Chunk{Type: ChunkToolOutputDenied, Fields: fields})
}

func (writer *Writer) WriteToolApprovalRequest(approvalID, callID, name string, input any) error {
	return WriteSSE(writer.w, Chunk{Type: ChunkToolApprovalRequest, Fields: map[string]any{
		"approvalId": approvalID, "toolCallId": callID, "toolName": name, "args": input,
	}})
}

type ApprovalResponseOpts struct {
	Reason           string
	ProviderExecuted *bool
	ProviderMetadata map[string]any
}

func (writer *Writer) WriteToolApprovalResponse(id string, approved bool, options *ApprovalResponseOpts) error {
	fields := map[string]any{"approvalId": id, "approved": approved}
	if options != nil {
		if options.Reason != "" {
			fields["reason"] = options.Reason
		}
		if options.ProviderExecuted != nil {
			fields["providerExecuted"] = *options.ProviderExecuted
		}
		fields = withProviderMetadata(fields, options.ProviderMetadata)
	}
	return WriteSSE(writer.w, Chunk{Type: ChunkToolApprovalResponse, Fields: fields})
}
