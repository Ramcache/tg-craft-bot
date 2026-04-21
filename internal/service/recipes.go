package service

import (
	"time"

	"tg-craft-bot/internal/domain"
)

type RecipeCatalog struct {
	byKey map[string]domain.Recipe
	list  []domain.Recipe
}

func NewRecipeCatalog() *RecipeCatalog {
	recipes := []domain.Recipe{
		// БАНКИ
		{
			Key:        "gpc",
			Name:       "Большое зелье спокойствия",
			MenuLabel:  "Банки антиагр х5 (15.000 ЗОЛОТО💰)",
			Type:       domain.RecipePotion,
			UnitName:   "зелья",
			Duration:   28*time.Hour + 40*time.Minute,
			DefaultQty: 5,
		},
		{
			Key:        "gec",
			Name:       "Большой эликсир очищения",
			MenuLabel:  "Банки очища х5 (15.000 ЗОЛОТО💰)",
			Type:       domain.RecipeElixir,
			UnitName:   "эликсира",
			Duration:   20*time.Hour + 51*time.Minute,
			DefaultQty: 5,
		},
		{
			Key:        "gag",
			Name:       "Большое зелье гнева",
			MenuLabel:  "Банки агра х5 (15.000 ЗОЛОТО💰)",
			Type:       domain.RecipePotion,
			UnitName:   "зелья",
			Duration:   28*time.Hour + 40*time.Minute,
			DefaultQty: 5,
		},
		{
			Key:        "gdo",
			Name:       "Большое зелье обнаружения",
			MenuLabel:  "Банки антиинвиза х5 (15.000 ЗОЛОТО💰)",
			Type:       domain.RecipePotion,
			UnitName:   "зелья",
			Duration:   28*time.Hour + 40*time.Minute,
			DefaultQty: 5,
		},
		{
			Key:        "gls",
			Name:       "Большое зелье силы жизни",
			MenuLabel:  "Банки хила х5 (15.000 ЗОЛОТО💰)",
			Type:       domain.RecipePotion,
			UnitName:   "зелья",
			Duration:   28*time.Hour + 50*time.Minute,
			DefaultQty: 5,
		},
		{
			Key:        "gts",
			Name:       "Большой эликсир странствующего духа",
			MenuLabel:  "Банки тп х5 (15.000 ЗОЛОТО💰)",
			Type:       domain.RecipeElixir,
			UnitName:   "эликсиров",
			Duration:   20*time.Hour + 51*time.Minute,
			DefaultQty: 5,
		},

		// СВИТКИ
		{
			Key:        "gsh",
			Name:       "Великолепный свиток несокрушимости",
			MenuLabel:  "Свитки сопры х5 (50.000 ЗОЛОТО💰)",
			Type:       domain.RecipeScroll,
			UnitName:   "свитков",
			Duration:   118 * time.Hour,
			DefaultQty: 5,
		},
		{
			Key:        "gss",
			Name:       "Великолепный свиток бессмертия",
			MenuLabel:  "Свитки хп х5 (50.000 ЗОЛОТО💰)",
			Type:       domain.RecipeScroll,
			UnitName:   "свитков",
			Duration:   118 * time.Hour,
			DefaultQty: 5,
		},
		{
			Key:        "gsm",
			Name:       "Великолепный свиток могущества",
			MenuLabel:  "Свитки маны х5 (50.000 ЗОЛОТО💰)",
			Type:       domain.RecipeScroll,
			UnitName:   "свитков",
			Duration:   118 * time.Hour,
			DefaultQty: 5,
		},
		{
			Key:        "test",
			Name:       "test",
			MenuLabel:  "test(50.000 ЗОЛОТО💰)",
			Type:       domain.RecipeScroll,
			UnitName:   "свитков",
			Duration:   5 * time.Second,
			DefaultQty: 5,
		},

		// Только для админа
		{
			Key:        "vlp",
			Name:       "Великолепный лист пергамента",
			MenuLabel:  "великолепный лист пергамента ×1",
			Type:       domain.RecipePotion,
			UnitName:   "лист",
			Duration:   13*time.Hour + 13*time.Minute,
			DefaultQty: 1,
			AdminOnly:  true,
		},
		{
			Key:        "gbf",
			Name:       "Большой флакон",
			MenuLabel:  "большой флакон ×1",
			Type:       domain.RecipePotion,
			UnitName:   "флакон",
			Duration:   2*time.Hour + 46*time.Minute,
			DefaultQty: 1,
			AdminOnly:  true,
		},
	}

	byKey := make(map[string]domain.Recipe, len(recipes))
	for _, r := range recipes {
		byKey[r.Key] = r
	}

	return &RecipeCatalog{
		byKey: byKey,
		list:  recipes,
	}
}

func (c *RecipeCatalog) Get(key string) (domain.Recipe, bool) {
	r, ok := c.byKey[key]
	return r, ok
}

func (c *RecipeCatalog) List() []domain.Recipe {
	out := make([]domain.Recipe, 0, len(c.list))
	out = append(out, c.list...)
	return out
}
