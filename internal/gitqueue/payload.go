package gitqueue

// Payload is the encrypted task sent from sender to receiver.
type Payload struct {
	Version           int    `json:"version"`
	TaskID            string `json:"task_id"`
	SenderHostname    string `json:"sender_hostname"`
	RecipientHostname string `json:"recipient_hostname"`
	Repo              string `json:"repo"`
	Message           string `json:"message,omitempty"`
	Timestamp         int64  `json:"timestamp"`
	Screenshot        struct {
		Name     string `json:"name"`
		DataB64  string `json:"data_b64"`
	} `json:"screenshot"`
}

// ResultPayload is the encrypted result sent from receiver back to sender.
type ResultPayload struct {
	Version           int    `json:"version"`
	TaskID            string `json:"task_id"`
	SenderHostname    string `json:"sender_hostname"`
	RecipientHostname string `json:"recipient_hostname"`
	Status            string `json:"status"` // "success" or "error"
	PRUrl             string `json:"pr_url,omitempty"`
	Branch            string `json:"branch,omitempty"`
	Summary           string `json:"summary"`
	Timestamp         int64  `json:"timestamp"`
}
