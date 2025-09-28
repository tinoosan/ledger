package v1

import (
	"github.com/tinoosan/ledger/internal/dictionary"
	"github.com/tinoosan/ledger/internal/ledger"
	"net/http"
)

// GET /v1/dictionary/groups?type=
func (s *Server) getGroupsDictionary(w http.ResponseWriter, r *http.Request) {
	var t *ledger.AccountType
	if ts := r.URL.Query().Get("type"); ts != "" {
		tt := ledger.AccountType(ts)
		t = &tt
	}
	// Build response grouped by type
	types := []ledger.AccountType{ledger.AccountTypeAsset, ledger.AccountTypeLiability, ledger.AccountTypeEquity, ledger.AccountTypeRevenue, ledger.AccountTypeExpense}
	type groupItem struct {
		Type   ledger.AccountType    `json:"type"`
		Groups []dictionary.GroupDef `json:"groups"`
	}
	out := struct {
		Items []groupItem `json:"items"`
	}{Items: []groupItem{}}
	for _, typ := range types {
		if t != nil && *t != typ {
			continue
		}
		out.Items = append(out.Items, groupItem{Type: typ, Groups: dictionary.GroupsFor(&typ)})
	}
	toJSON(w, http.StatusOK, out)
}
