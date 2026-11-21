package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jsonData := `[
			{
				"authAlert": {
					"emailTemplate": {
						"body": "<p>Hello,</p>\n<p>We noticed a login to your {APP_NAME} account from a new location:</p>\n<p><em>{ALERT_INFO}</em></p>\n<p><strong>If this wasn't you, you should immediately change your {APP_NAME} account password to revoke access from all other locations.</strong></p>\n<p>If this was you, you may disregard this email.</p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
						"subject": "Login from a new location"
					},
					"enabled": true
				},
				"authRule": "",
				"authToken": {
					"duration": 604800
				},
				"confirmEmailChangeTemplate": {
					"body": "<p>Hello,</p>\n<p>Click on the button below to confirm your new email address.</p>\n<p>\n  <a class=\"btn\" href=\"{APP_URL}/_/#/auth/confirm-email-change/{TOKEN}\" target=\"_blank\" rel=\"noopener\">Confirm new email</a>\n</p>\n<p><i>If you didn't ask to change your email address, you can ignore this email.</i></p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
					"subject": "Confirm your {APP_NAME} new email address"
				},
				"createRule": null,
				"deleteRule": null,
				"emailChangeToken": {
					"duration": 1800
				},
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"help": "",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cost": 0,
						"help": "",
						"hidden": true,
						"id": "password901924565",
						"max": 0,
						"min": 8,
						"name": "password",
						"pattern": "",
						"presentable": false,
						"required": true,
						"system": true,
						"type": "password"
					},
					{
						"autogeneratePattern": "[a-zA-Z0-9]{50}",
						"help": "",
						"hidden": true,
						"id": "text2504183744",
						"max": 60,
						"min": 30,
						"name": "tokenKey",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"exceptDomains": [],
						"help": "",
						"hidden": false,
						"id": "email3885137012",
						"name": "email",
						"onlyDomains": [],
						"presentable": false,
						"required": true,
						"system": true,
						"type": "email"
					},
					{
						"help": "",
						"hidden": false,
						"id": "bool1547992806",
						"name": "emailVisibility",
						"presentable": false,
						"required": false,
						"system": true,
						"type": "bool"
					},
					{
						"help": "",
						"hidden": false,
						"id": "bool256245529",
						"name": "verified",
						"presentable": false,
						"required": false,
						"system": true,
						"type": "bool"
					},
					{
						"autogeneratePattern": "",
						"help": "",
						"hidden": false,
						"id": "text1579384326",
						"max": 255,
						"min": 0,
						"name": "name",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"help": "",
						"hidden": false,
						"id": "file376926767",
						"maxSelect": 1,
						"maxSize": 0,
						"mimeTypes": [
							"image/jpeg",
							"image/png",
							"image/svg+xml",
							"image/gif",
							"image/webp"
						],
						"name": "avatar",
						"presentable": false,
						"protected": false,
						"required": false,
						"system": false,
						"thumbs": [],
						"type": "file"
					},
					{
						"cascadeDelete": false,
						"collectionId": "pbc_1470537641",
						"help": "",
						"hidden": false,
						"id": "relation3839242249",
						"maxSelect": 100,
						"minSelect": 0,
						"name": "allowed_models",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"help": "Maximum total tokens per UTC week (resets Monday 00:00 UTC); 0 means unlimited.",
						"hidden": false,
						"id": "number1713851962",
						"max": null,
						"min": 0,
						"name": "weekly_token_quota",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "",
						"hidden": false,
						"id": "json3846545605",
						"maxSize": 1048576,
						"name": "settings",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "json"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"fileToken": {
					"duration": 180
				},
				"id": "_pb_users_auth_",
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `idx_tokenKey__pb_users_auth_` + "`" + ` ON ` + "`" + `users` + "`" + ` (` + "`" + `tokenKey` + "`" + `)",
					"CREATE UNIQUE INDEX ` + "`" + `idx_email__pb_users_auth_` + "`" + ` ON ` + "`" + `users` + "`" + ` (` + "`" + `email` + "`" + `) WHERE ` + "`" + `email` + "`" + ` != ''",
					"CREATE UNIQUE INDEX ` + "`" + `idx_users_name` + "`" + ` ON ` + "`" + `users` + "`" + ` (name) WHERE name != ''"
				],
				"listRule": null,
				"manageRule": null,
				"mfa": {
					"duration": 600,
					"enabled": false,
					"rule": ""
				},
				"name": "users",
				"oauth2": {
					"enabled": false,
					"mappedFields": {
						"avatarURL": "avatar",
						"id": "",
						"name": "name",
						"username": ""
					},
					"providers": []
				},
				"otp": {
					"duration": 180,
					"emailTemplate": {
						"body": "<p>Hello,</p>\n<p>Your one-time password is: <strong>{OTP}</strong></p>\n<p><i>If you didn't ask for the one-time password, you can ignore this email.</i></p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
						"subject": "OTP for {APP_NAME}"
					},
					"enabled": false,
					"length": 8
				},
				"passwordAuth": {
					"enabled": true,
					"identityFields": [
						"email"
					]
				},
				"passwordResetToken": {
					"duration": 1800
				},
				"resetPasswordTemplate": {
					"body": "<p>Hello,</p>\n<p>Click on the button below to reset your password.</p>\n<p>\n  <a class=\"btn\" href=\"{APP_URL}/_/#/auth/confirm-password-reset/{TOKEN}\" target=\"_blank\" rel=\"noopener\">Reset password</a>\n</p>\n<p><i>If you didn't ask to reset your password, you can ignore this email.</i></p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
					"subject": "Reset your {APP_NAME} password"
				},
				"system": false,
				"type": "auth",
				"updateRule": null,
				"verificationTemplate": {
					"body": "<p>Hello,</p>\n<p>Thank you for joining us at {APP_NAME}.</p>\n<p>Click on the button below to verify your email address.</p>\n<p>\n  <a class=\"btn\" href=\"{APP_URL}/_/#/auth/confirm-verification/{TOKEN}\" target=\"_blank\" rel=\"noopener\">Verify</a>\n</p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
					"subject": "Verify your {APP_NAME} email"
				},
				"verificationToken": {
					"duration": 259200
				},
				"viewRule": null
			},
			{
				"createRule": null,
				"deleteRule": null,
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"help": "",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"help": "",
						"hidden": false,
						"id": "text1579384326",
						"max": 64,
						"min": 0,
						"name": "name",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"help": "",
						"hidden": false,
						"id": "text2651864036",
						"max": 512,
						"min": 0,
						"name": "base_url",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"help": "Upstream API key (masked in UI). Leave blank to send no API key.",
						"hidden": false,
						"id": "text2813317029",
						"max": 8192,
						"min": 0,
						"name": "api_key",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"help": "",
						"hidden": false,
						"id": "bool1358543748",
						"name": "enabled",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "bool"
					}
				],
				"id": "pbc_1120360670",
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `idx_upstream_provider_name` + "`" + ` ON ` + "`" + `upstream_providers` + "`" + ` (name)"
				],
				"listRule": null,
				"name": "upstream_providers",
				"system": false,
				"type": "base",
				"updateRule": null,
				"viewRule": null
			},
			{
				"createRule": null,
				"deleteRule": null,
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"help": "",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cascadeDelete": true,
						"collectionId": "pbc_1120360670",
						"help": "",
						"hidden": false,
						"id": "relation2462348188",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "provider",
						"presentable": false,
						"required": true,
						"system": false,
						"type": "relation"
					},
					{
						"autogeneratePattern": "",
						"help": "",
						"hidden": false,
						"id": "text3578368839",
						"max": 128,
						"min": 0,
						"name": "display_name",
						"pattern": "",
						"presentable": true,
						"primaryKey": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"help": "",
						"hidden": false,
						"id": "text1579384326",
						"max": 128,
						"min": 0,
						"name": "name",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"help": "Smaller values are tried first (e.g. 10 before 100).",
						"hidden": false,
						"id": "number1655102503",
						"max": null,
						"min": null,
						"name": "priority",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "",
						"hidden": false,
						"id": "number1007202954",
						"max": null,
						"min": null,
						"name": "context_window",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "",
						"hidden": false,
						"id": "number4129561874",
						"max": null,
						"min": null,
						"name": "max_output_tokens",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "",
						"hidden": false,
						"id": "bool1358543748",
						"name": "enabled",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "bool"
					},
					{
						"help": "Cooldown deadline (UTC); empty when available or blocked by auth error.",
						"hidden": false,
						"id": "date866445133",
						"max": "",
						"min": "",
						"name": "unavailable_until",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "date"
					},
					{
						"help": "Reason the model was marked unavailable; clear to manually restore.",
						"hidden": false,
						"id": "select1205397881",
						"maxSelect": 1,
						"name": "unavailable_reason",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "select",
						"values": [
							"quota",
							"rate_limit",
							"auth"
						]
					},
					{
						"autogeneratePattern": "",
						"help": "Latest upstream error summary recorded when marking unavailable.",
						"hidden": false,
						"id": "text1066830442",
						"max": 1024,
						"min": 0,
						"name": "last_error",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "pbc_1470537641",
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `idx_upstream_model_provider_name` + "`" + ` ON ` + "`" + `upstream_models` + "`" + ` (provider, name)"
				],
				"listRule": null,
				"name": "upstream_models",
				"system": false,
				"type": "base",
				"updateRule": null,
				"viewRule": null
			},
			{
				"createRule": null,
				"deleteRule": null,
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"help": "",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cascadeDelete": true,
						"collectionId": "_pb_users_auth_",
						"help": "",
						"hidden": false,
						"id": "relation2375276105",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "user",
						"presentable": false,
						"required": true,
						"system": false,
						"type": "relation"
					},
					{
						"autogeneratePattern": "",
						"help": "",
						"hidden": false,
						"id": "text2462348188",
						"max": 64,
						"min": 0,
						"name": "provider",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"help": "",
						"hidden": false,
						"id": "text3616895705",
						"max": 128,
						"min": 0,
						"name": "model",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"help": "UTC date in YYYY-MM-DD format.",
						"hidden": false,
						"id": "text2862495610",
						"max": 10,
						"min": 10,
						"name": "date",
						"pattern": "^\\d{4}-\\d{2}-\\d{2}$",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"help": "",
						"hidden": false,
						"id": "number2072368721",
						"max": null,
						"min": 0,
						"name": "requests",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "",
						"hidden": false,
						"id": "number2356723103",
						"max": null,
						"min": 0,
						"name": "prompt_tokens",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "",
						"hidden": false,
						"id": "number3312136409",
						"max": null,
						"min": 0,
						"name": "completion_tokens",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "",
						"hidden": false,
						"id": "number1649291928",
						"max": null,
						"min": 0,
						"name": "total_tokens",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "pbc_2804069769",
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `idx_daily_model_usage_unique` + "`" + ` ON ` + "`" + `daily_model_usage` + "`" + ` (` + "`" + `user` + "`" + `, ` + "`" + `provider` + "`" + `, ` + "`" + `model` + "`" + `, ` + "`" + `date` + "`" + `)",
					"CREATE INDEX ` + "`" + `idx_daily_model_usage_user_date` + "`" + ` ON ` + "`" + `daily_model_usage` + "`" + ` (` + "`" + `user` + "`" + `, ` + "`" + `date` + "`" + `)"
				],
				"listRule": null,
				"name": "daily_model_usage",
				"system": false,
				"type": "base",
				"updateRule": null,
				"viewRule": null
			},
			{
				"createRule": null,
				"deleteRule": null,
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"help": "",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "[A-Z0-9]{8}",
						"help": "",
						"hidden": false,
						"id": "text1997877400",
						"max": 64,
						"min": 4,
						"name": "code",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"help": "",
						"hidden": false,
						"id": "bool1358543748",
						"name": "enabled",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "bool"
					},
					{
						"help": "Maximum registrations allowed; 0 means unlimited.",
						"hidden": false,
						"id": "number2247082791",
						"max": null,
						"min": 0,
						"name": "max_uses",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "",
						"hidden": false,
						"id": "number1662326478",
						"max": null,
						"min": 0,
						"name": "used_count",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "Optional UTC expiration time; empty means never expires.",
						"hidden": false,
						"id": "date261981154",
						"max": "",
						"min": "",
						"name": "expires_at",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "date"
					},
					{
						"cascadeDelete": false,
						"collectionId": "pbc_1470537641",
						"help": "",
						"hidden": false,
						"id": "relation1813892085",
						"maxSelect": 100,
						"minSelect": 0,
						"name": "allowed_models",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"help": "Maximum total tokens per UTC week assigned to new users; 0 means unlimited.",
						"hidden": false,
						"id": "number1713851962",
						"max": null,
						"min": 0,
						"name": "weekly_token_quota",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"help": "Default user settings (blacklist, session) copied to new users.",
						"hidden": false,
						"id": "json3846545605",
						"maxSize": 1048576,
						"name": "settings",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "json"
					},
					{
						"autogeneratePattern": "",
						"help": "",
						"hidden": false,
						"id": "text3485334036",
						"max": 255,
						"min": 0,
						"name": "note",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "pbc_3208698804",
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `idx_invite_codes_code` + "`" + ` ON ` + "`" + `invite_codes` + "`" + ` (` + "`" + `code` + "`" + `)"
				],
				"listRule": null,
				"name": "invite_codes",
				"system": false,
				"type": "base",
				"updateRule": null,
				"viewRule": null
			}
		]`

		return app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false)
	}, func(app core.App) error {
		return nil
	})
}
