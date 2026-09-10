package mailservice

import (
	"io"

	"bearstack/internal/document"
	"bearstack/internal/mailimport"
)

type mailbox interface {
	Logout() error
	UndeletedUIDs() ([]uint32, error)
	FetchMessage(uint32) (io.Reader, error)
	DeleteMessage(uint32) error
}
type imapMailbox struct{ client *mailimport.Client }

func openMailbox(settings document.MailImportSettings, readOnly bool) (mailbox, error) {
	client, err := mailimport.OpenMailbox(settings, readOnly)
	if err != nil {
		return nil, err
	}
	return imapMailbox{client: client}, nil
}
func (m imapMailbox) Logout() error                    { return m.client.Logout() }
func (m imapMailbox) UndeletedUIDs() ([]uint32, error) { return mailimport.UndeletedUIDs(m.client) }
func (m imapMailbox) FetchMessage(uid uint32) (io.Reader, error) {
	return mailimport.FetchMessage(m.client, uid)
}
func (m imapMailbox) DeleteMessage(uid uint32) error { return mailimport.DeleteMessage(m.client, uid) }
