package app

import (
	"ssh-connect/internal/service"
)

func (a *App) PreviewHostSnippetImport(content string) (service.HostSnippetPreview, error) {
	svc, err := a.service()
	if err != nil {
		return service.HostSnippetPreview{}, err
	}
	return svc.PreviewHostSnippetImport(a.ctx, content)
}

func (a *App) ImportHostSnippet(input service.HostSnippetImportInput) (service.HostSnippetImportResult, error) {
	svc, err := a.service()
	if err != nil {
		return service.HostSnippetImportResult{}, err
	}
	return svc.ImportHostSnippet(a.ctx, input)
}
