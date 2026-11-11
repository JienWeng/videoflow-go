package api

import (
	"context"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

type mediaRoute struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Endpoint string `json:"endpoint"`
}
type mediaRoutesKey struct{}

func (s *Server) captureMediaRoutes() (map[string]mediaRoute, error) {
	routes := map[string]mediaRoute{}
	selections, err := s.routes().MediaSelections()
	if err != nil {
		return nil, err
	}
	for _, kind := range []string{"image", "reference", "video"} {
		choice := selections[kind]
		provider, model := choice.Provider, choice.Model
		_, endpoint, err := s.routes().Media(provider, model, "")
		if err != nil {
			return nil, err
		}
		routes[kind] = mediaRoute{provider, model, endpoint}
	}
	return routes, nil
}
func (s *Server) selectedMedia(ctx context.Context, kind string) (providers.MediaClient, string, string, error) {
	if routes, ok := ctx.Value(mediaRoutesKey{}).(map[string]mediaRoute); ok {
		route := routes[kind]
		client, _, err := s.routes().Media(route.Provider, route.Model, route.Endpoint)
		return client, route.Provider, route.Model, err
	}
	return s.routes().SelectedMedia(kind)
}
func (s *Server) prepareVideoJob(job *models.RenderJob) error {
	if job.Provider == "" {
		job.Provider, job.Model = s.routes().MediaSelection("video")
	}
	if job.Model == "" {
		job.Model = s.routes().DefaultMediaModel(job.Provider, "video")
	}
	_, endpoint, err := s.routes().Media(job.Provider, job.Model, job.ProviderEndpoint)
	if err != nil {
		return err
	}
	job.ProviderEndpoint = endpoint
	return nil
}
