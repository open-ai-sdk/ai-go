import { useState } from 'react';
import { createRoot } from 'react-dom/client';
import {
  DefaultChatTransport,
  isToolUIPart,
  lastAssistantMessageIsCompleteWithApprovalResponses,
  lastAssistantMessageIsCompleteWithToolCalls,
  type UIMessage,
} from 'ai';
import { useChat } from '@ai-sdk/react';

type ConformanceTools = {
  echo: {
    input: { value: string };
    output: { result: string };
  };
  dangerous_action: {
    input: { target: string };
    output: { ok: boolean };
  };
};

type ConformanceMessage = UIMessage<
  unknown,
  Record<string, unknown>,
  ConformanceTools
>;

const scenario =
  new URLSearchParams(window.location.search).get('scenario') ?? 'text';

function App() {
  const [input, setInput] = useState('hello');
  const {
    addToolApprovalResponse,
    addToolOutput,
    error,
    messages,
    sendMessage,
    status,
  } = useChat<ConformanceMessage>({
    transport: new DefaultChatTransport({
      api: `/chat?scenario=${encodeURIComponent(scenario)}`,
    }),
    sendAutomaticallyWhen: options =>
      lastAssistantMessageIsCompleteWithApprovalResponses(options) ||
      lastAssistantMessageIsCompleteWithToolCalls(options),
  });

  return (
    <main>
      <form
        onSubmit={event => {
          event.preventDefault();
          void sendMessage({ text: input });
        }}
      >
        <input
          aria-label="Message"
          value={input}
          onChange={event => setInput(event.target.value)}
        />
        <button type="submit">Send</button>
      </form>

      <output data-testid="status">{status}</output>
      {error && <p role="alert">{error.message}</p>}

      <section data-testid="messages">
        {messages.map(message => (
          <article key={message.id} data-role={message.role}>
            {message.parts.map((part, index) => {
              if (part.type === 'text') {
                return <p key={index}>{part.text}</p>;
              }
              if (isToolUIPart(part)) {
                const tool = part;
                return (
                  <div key={index} data-testid={`tool-${tool.state}`}>
                    <span>{tool.type}</span>
                    {tool.state === 'input-available' && (
                      <button
                        onClick={() =>
                          void addToolOutput({
                            tool: 'echo',
                            toolCallId: tool.toolCallId,
                            output: { result: 'tool-ok' },
                          })
                        }
                      >
                        Run tool
                      </button>
                    )}
                    {tool.state === 'approval-requested' && tool.approval && (
                      <>
                        <button
                          onClick={() =>
                            void addToolApprovalResponse({
                              id: tool.approval!.id,
                              approved: true,
                            })
                          }
                        >
                          Approve
                        </button>
                        <button
                          onClick={() =>
                            void addToolApprovalResponse({
                              id: tool.approval!.id,
                              approved: false,
                              reason: 'user denied',
                            })
                          }
                        >
                          Deny
                        </button>
                      </>
                    )}
                  </div>
                );
              }
              return null;
            })}
          </article>
        ))}
      </section>
    </main>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
