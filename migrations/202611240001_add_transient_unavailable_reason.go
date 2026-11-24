package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		return updateUnavailableReasonValues(app, true)
	}, func(app core.App) error {
		return updateUnavailableReasonValues(app, false)
	})
}

func updateUnavailableReasonValues(app core.App, addTransient bool) error {
	col, err := app.FindCollectionByNameOrId("upstream_models")
	if err != nil {
		return err
	}
	field, ok := col.Fields.GetByName("unavailable_reason").(*core.SelectField)
	if !ok || field == nil {
		return nil
	}

	const transientValue = "transient"
	hasTransient := slices.Contains(field.Values, transientValue)
	if addTransient && !hasTransient {
		field.Values = append(field.Values, transientValue)
		return app.Save(col)
	}
	if !addTransient && hasTransient {
		field.Values = slices.DeleteFunc(field.Values, func(v string) bool {
			return v == transientValue
		})
		return app.Save(col)
	}
	return nil
}
