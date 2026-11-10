package audit

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrXMLInvalido indica que o arquivo não é um XML de NF-e que a API aceita.
var ErrXMLInvalido = errors.New("xml de NF-e inválido")

// ExtrairChaveAcesso lê o XML em streaming e devolve a chave de acesso
// (44 dígitos) do atributo Id da tag <infNFe>, ex.: Id="NFe4225...".
//
// A API faz só essa checagem mínima. As regras fiscais (CFOP, NCM, totais,
// dígito verificador) ficam no worker — a API não precisa conhecê-las.
//
// Usamos xml.Decoder (token a token) em vez de xml.Unmarshal num struct
// gigante porque só precisamos de um atributo e queremos garantir que o
// documento inteiro está bem formado.
func ExtrairChaveAcesso(dados []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(dados))
	var chave string

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%w: xml mal formado: %v", ErrXMLInvalido, err)
		}
		inicio, ok := tok.(xml.StartElement)
		if !ok || inicio.Name.Local != "infNFe" || chave != "" {
			continue
		}
		for _, attr := range inicio.Attr {
			if attr.Name.Local == "Id" {
				chave = strings.TrimPrefix(attr.Value, "NFe")
			}
		}
	}

	if chave == "" {
		return "", fmt.Errorf("%w: tag infNFe com atributo Id não encontrada", ErrXMLInvalido)
	}
	if !somenteDigitos(chave, 44) {
		return "", fmt.Errorf("%w: chave de acesso deve ter 44 dígitos, recebido %q", ErrXMLInvalido, chave)
	}
	return chave, nil
}

func somenteDigitos(s string, tamanho int) bool {
	if len(s) != tamanho {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
