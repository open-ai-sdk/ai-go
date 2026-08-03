package aisdkhttp

import (
	"iter"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func sequenceFromChannel(events <-chan aikit.StepEvent) iter.Seq2[aikit.StepEvent, error] {
	return func(yield func(aikit.StepEvent, error) bool) {
		for event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func eventSequence(events ...aikit.StepEvent) iter.Seq2[aikit.StepEvent, error] {
	return func(yield func(aikit.StepEvent, error) bool) {
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}
}
