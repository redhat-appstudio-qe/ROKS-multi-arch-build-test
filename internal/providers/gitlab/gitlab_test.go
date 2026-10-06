package gitlab

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/konflux-test/internal/providers"
	gl "github.com/xanzy/go-gitlab"
)

type fakeClient struct {
	project          *gl.Project
	getProjectErrors []error
	updateErrors     []error
	getCalls         int
	updateCalls      int
}

func (f *fakeClient) GetProject(string) (*gl.Project, error) {
	f.getCalls++
	if len(f.getProjectErrors) > 0 {
		err := f.getProjectErrors[0]
		f.getProjectErrors = f.getProjectErrors[1:]
		return nil, err
	}
	if f.project == nil {
		return nil, errors.New("not found")
	}
	return f.project, nil
}

func (f *fakeClient) GetFileMetaData(string, string, string) (*gl.File, error) {
	return &gl.File{CommitID: "sha-1"}, nil
}

func (f *fakeClient) GetFile(string, string, string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte("FROM base\n")), nil
}

func (f *fakeClient) UpdateFile(string, string, string, string, string, string) error {
	f.updateCalls++
	if len(f.updateErrors) > 0 {
		err := f.updateErrors[0]
		f.updateErrors = f.updateErrors[1:]
		return err
	}
	return nil
}

var _ = Describe("GitLab provider", func() {
	It("reuses canonical project", func() {
		client := &fakeClient{project: &gl.Project{Path: "dr_test_mathwizz_gl", PathWithNamespace: "konflux-qe/dr_test_mathwizz_gl", WebURL: CanonicalFixtureURL}}
		got, err := New(client).ValidateFixture(context.Background(), providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl"})
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
		want := providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl", URL: CanonicalFixtureURL}
		Expect(got).To(Equal(want))
		Expect(client.getCalls).To(Equal(1))
	})

	It("retries project validation after a 5xx response", func() {
		client := &fakeClient{
			project:          &gl.Project{PathWithNamespace: "konflux-qe/dr_test_mathwizz_gl"},
			getProjectErrors: []error{&gl.ErrorResponse{Response: &http.Response{StatusCode: http.StatusBadGateway}}},
		}
		adapter := NewWithRetry(client, providers.RetryPolicy{MaxRetries: 3, Sleep: func(context.Context, time.Duration) error { return nil }})

		_, err := adapter.ValidateFixture(context.Background(), providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl"})
		Expect(err).NotTo(HaveOccurred())
		Expect(client.getCalls).To(Equal(2))
	})

	It("rejects non-canonical project without creation", func() {
		client := &fakeClient{}
		_, err := New(client).ValidateFixture(context.Background(), providers.FixtureRepository{Owner: "other", Name: "dr_test_mathwizz_gl"})
		Expect(err).To(HaveOccurred())
		Expect(client.getCalls).To(Equal(0))
		Expect(client.updateCalls).To(Equal(0))
	})

	It("reads and updates files in canonical project", func() {
		client := &fakeClient{}
		adapter := New(client)
		fixture := providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl", URL: CanonicalFixtureURL}
		file, err := adapter.ReadFile(context.Background(), fixture, "web-server/Dockerfile", "main")
		Expect(err).NotTo(HaveOccurred())
		Expect(file.SHA).To(Equal("sha-1"))
		Expect(file.Content).To(Equal("FROM base\n"))
		_, err = adapter.UpdateFile(context.Background(), fixture, "web-server/Dockerfile", "main", "content", "sha-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(client.updateCalls).To(Equal(1))
	})

	It("retries file updates after a 5xx response", func() {
		client := &fakeClient{
			updateErrors: []error{&gl.ErrorResponse{Response: &http.Response{StatusCode: http.StatusBadGateway}}},
		}
		adapter := NewWithRetry(client, providers.RetryPolicy{MaxRetries: 3, Sleep: func(context.Context, time.Duration) error { return nil }})
		fixture := providers.FixtureRepository{Owner: "konflux-qe", Name: "dr_test_mathwizz_gl", URL: CanonicalFixtureURL}

		_, err := adapter.UpdateFile(context.Background(), fixture, "web-server/Dockerfile", "main", "content", "sha-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(client.updateCalls).To(Equal(2))
	})
})
