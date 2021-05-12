package controller

var authMap = map[string]string{
	"GetWalletByName":          "admin",
	"SaveAddress":              "admin",
	"ForbiddenAddress":         "admin",
	"RepublishMessage":         "admin",
	"ListBlockedMessage":       "admin",
	"GetMessageByUid":          "read",
	"HasWalletAddress":         "read",
	"PushMessageWithId":        "write",
	"GetWalletAddress":         "admin",
	"GetFeeConfig":             "admin",
	"UpdateNonce":              "admin",
	"ListAddress":              "admin",
	"WaitMessage":              "read",
	"UpdateWallet":             "admin",
	"GetAddress":               "admin",
	"GetGlobalFeeConfig":       "admin",
	"UpdateFilledMessageByID":  "admin",
	"HasWallet":                "admin",
	"ListWallet":               "admin",
	"PushMessage":              "write",
	"GetMessageBySignedCid":    "read",
	"GetMessageByFromAndNonce": "read",
	"MarkBadMessage":           "admin",
	"DeleteWallet":             "admin",
	"GetNode":                  "admin",
	"HasMessageByUid":          "read",
	"SaveNode":                 "admin",
	"UpdateAllFilledMessage":   "admin",
	"HasFeeConfig":             "admin",
	"GetMessageByUnsignedCid":  "read",
	"SetSharedParams":          "admin",
	"ListNode":                 "admin",
	"DeleteNode":               "admin",
	"ListFeeConfig":            "admin",
	"DeleteFeeConfig":          "admin",
	"UpdateMessageStateByID":   "admin",
	"ListFailedMessage":        "admin",
	"DeleteAddress":            "admin",
	"GetSharedParams":          "admin",
	"HasNode":                  "admin",
	"SetSelectMsgNum":          "admin",
	"GetMessageByCid":          "read",
	"ListMessageByFromState":   "admin",
	"ListMessageByAddress":     "admin",
	"ListRemoteWalletAddress":  "admin",
	"SaveFeeConfig":            "admin",
	"ListMessage":              "admin",
	"ReplaceMessage":           "admin",
	"GetWalletByID":            "admin",
	"HasAddress":               "admin",
	"ActiveAddress":            "admin",
	"ListWalletAddress":        "admin",
	"GetWalletFeeConfig":       "admin",
	"UpdateMessageStateByCid":  "admin",
	"SaveWallet":               "admin",
	"RefreshSharedParams":      "admin",
}
