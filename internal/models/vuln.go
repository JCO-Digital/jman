package models

import "encoding/json"

type Cwe struct {
	Cwe         string `json:"cwe"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Cvss struct {
	Version     string  `json:"version"`
	Vector      string  `json:"vector"`
	Av          string  `json:"av"`
	Ac          string  `json:"ac"`
	Pr          string  `json:"pr"`
	Ui          string  `json:"ui"`
	S           string  `json:"s"`
	C           string  `json:"c"`
	I           string  `json:"i"`
	A           string  `json:"a"`
	Score       string  `json:"score"`
	Severity    string  `json:"severity"`
	Exploitable string  `json:"exploitable"`
	Impact      *string `json:"impact"`
}

type Impact struct {
	Cvss *Cvss `json:"cvss,omitempty"`
	Cwe  []Cwe `json:"cwe,omitempty"`
}

// UnmarshalJSON custom unmarshaler for Impact to handle empty arrays.
func (i *Impact) UnmarshalJSON(data []byte) error {
	if string(data) == "[]" || string(data) == "null" {
		return nil
	}
	type Alias Impact
	aux := (*Alias)(i)
	return json.Unmarshal(data, aux)
}

type Source struct {
	Id          string  `json:"id"`
	Name        string  `json:"name"`
	Link        string  `json:"link"`
	Description *string `json:"description"`
	Date        *string `json:"date"`
}

type Operator struct {
	MinVersion  *string `json:"min_version"`
	MinOperator *string `json:"min_operator"`
	MaxVersion  *string `json:"max_version"`
	MaxOperator *string `json:"max_operator"`
	Unfixed     string  `json:"unfixed"`
	Closed      string  `json:"closed"`
}

type Vulnerability struct {
	Uuid        string       `json:"uuid"`
	Name        string       `json:"name"`
	Description *string      `json:"description"`
	Operator    Operator     `json:"operator"`
	Source      []Source     `json:"source"`
	Impact      *Impact      `json:"impact,omitempty"`
	Sites       []PluginSite `json:"sites,omitempty"`
}

type VulnData struct {
	Name          *string         `json:"name"`
	Plugin        string          `json:"plugin"`
	Link          *string         `json:"link"`
	Latest        *string         `json:"latest"`
	Vulnerability []Vulnerability `json:"vulnerability"`
}

type VulnResponse struct {
	Error   int       `json:"error"`
	Message *string   `json:"message"`
	Data    *VulnData `json:"data,omitempty"`
	Updated any       `json:"updated"`
}

type VulnReport struct {
	Plugin        string        `json:"plugin"`
	Slug          string        `json:"slug"`
	PluginName    string        `json:"plugin_name"`
	Vulnerability Vulnerability `json:"vulnerability"`
	Sites         []PluginSite  `json:"sites"`
}

type VulnPlugin struct {
	PluginName    string          `json:"plugin_name"`
	Version       string          `json:"version"`
	Cvss          *float64        `json:"cvss"`
	Vulnerability []Vulnerability `json:"vulnerability"`
}
