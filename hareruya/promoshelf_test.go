package hareruya

import (
	"errors"
	"testing"

	"github.com/mtgban/go-mtgban/matcher"
)

// TestPromoShelf pins the retail promo shelf's newer wordings to the
// printings they name: the Wizards Play Network and Standard Showdown
// promos of 2025 and 2026, filed by year; a judge foil told apart by the
// year in its title; a prerelease card the set numbers among its own,
// which every set since Murders at Karlov Manor does; and the Spotlight
// Series shelf, which names a set of its own rather than a per-card row.
func TestPromoShelf(t *testing.T) {
	for _, tt := range []struct {
		desc, jp, en, card, foil string
		wantSet, wantNumber      string
	}{
		{
			desc: "a Final Fantasy Standard Showdown promo",
			jp:   "【Foil】■ボーダーレス■《SeeDの傭兵、スコール/Squall, SeeD Mercenary》(スタンダード・ショーダウン)[流星マーク] 金",
			en:   "【Foil】■Borderless■《Squall, SeeD Mercenary》[Showdown Promo]",
			card: "Squall, SeeD Mercenary", foil: "1",
			wantSet: "PSS5", wantNumber: "2",
		},
		{
			desc: "a 2026 commander event promo in the retro frame",
			jp:   "■旧枠■《彼方地のエルフ/Farhaven Elf》(コマンダーイベントプロモ)[流星マーク] 緑",
			en:   "■RetroF■《Farhaven Elf》[Commander Event Promo]",
			card: "Farhaven Elf", foil: "0",
			wantSet: "PW26", wantNumber: "2",
		},
		{
			desc: "a 2025 commander event promo in full art",
			jp:   "【Foil】■フルアート■《統率の塔/Command Tower》(コマンダーイベントプロモ)[流星マーク] 土地",
			en:   "【Foil】■Full-Art■《Command Tower》[Commander Event Promo]",
			card: "Command Tower", foil: "1",
			wantSet: "PW25", wantNumber: "17",
		},
		{
			desc: "a MagicCon full-art Counterspell is the second Universes Within number",
			jp:   "【Foil】(002)■フルアート■《対抗呪文/Counterspell》(MagicConプロモ)[流星マーク] 青",
			en:   "【Foil】(002)■Full-Art■《Counterspell》[MagicCon Promo]",
			card: "Counterspell", foil: "1",
			wantSet: "PURL", wantNumber: "2",
		},
		{
			desc: "a judge foil told apart by the year in its title",
			jp:   "【Foil】《吸血の教示者/Vampiric Tutor》(2018年版ジャッジ褒賞)[流星マーク] 黒",
			en:   "【Foil】■2018Ver.■《Vampiric Tutor》 [Judge Foil]",
			card: "Vampiric Tutor", foil: "1",
			wantSet: "J18", wantNumber: "2",
		},
		{
			desc: "a prerelease card the set numbers among its own",
			jp:   "【Foil】(431)《法の行使者、トミク/Tomik, Wielder of Law》(プレリリース)[MKM-P] 金",
			en:   "【Foil】(431)《Tomik, Wielder of Law》(Prerelease)[MKM-P]",
			card: "Tomik, Wielder of Law", foil: "1",
			wantSet: "MKM", wantNumber: "431",
		},
		{
			// The set holds a prerelease-tagged borderless at the very
			// number, and the promo line the date-stamped one the listing
			// sells; the title's number named the set's card and the two
			// aliased.
			desc: "a prerelease card whose set also numbers a prerelease at its number",
			jp:   "【Foil】(402)■ボーダーレス■《Delighted Halfling》(プレリリース)[LTR-P] 緑U",
			en:   "【Foil】(402)■Borderless■《Delighted Halfling》(Prerelease)[LTR-P]",
			card: "Delighted Halfling", foil: "1",
			wantSet: "PLTR", wantNumber: "402s",
		},
		{
			desc: "a prerelease card of a set that files them on its promo line",
			jp:   "【Foil】《Water Gun Balloon Game》(プレリリース)[UNF-P] 茶",
			en:   "【Foil】《Water Gun Balloon Game》[Prerelease]",
			card: "Water Gun Balloon Game", foil: "1",
			wantSet: "UNF", wantNumber: "538",
		},
		{
			desc: "the first Godzilla print, before the name was changed",
			jp:   "【Foil】(373a)■ゴジラ■《虚空の侵略者、スペースゴジラ/Spacegodzilla, Void Invader》[IKO-BF] 黒U",
			en:   "【Foil】■First edition■《Spacegodzilla, Void Invader》/《Void Beckoner》[IKO-BF]",
			card: "Void Beckoner", foil: "1",
			wantSet: "IKO", wantNumber: "373",
		},
		{
			desc: "a game day promo filed by its set's tag",
			jp:   "■テキストボックスレス■《傲慢な完全者/Imperious Perfect》(ゲームデー)[LRW-P] 緑U",
			en:   "《Imperious Perfect》[Game Day Promos]",
			card: "Imperious Perfect", foil: "0",
			wantSet: "PCMP", wantNumber: "9",
		},
		{
			desc: "the Spotlight Series shelf resolves by treatment, not a per-card row",
			jp:   "■ボーダーレス■《ミッドガルの傭兵、クラウド/Cloud, Midgar Mercenary》(スポットライトシリーズプロモ)[流星マーク] 白",
			en:   "■Borderless■《Cloud, Midgar Mercenary》[Spotlight Series Promo]",
			card: "Cloud, Midgar Mercenary", foil: "0",
			wantSet: "PSPL", wantNumber: "4",
		},
		{
			desc: "and so does a second card on the same shelf, a different treatment",
			jp:   "■拡張アート■《暗闇のなぞなぞ勝負/Riddles in the Dark》(スポットライトシリーズプロモ)[流星マーク] 青",
			en:   "■Extended Art■《Riddles in the Dark》[Spotlight Series Promo]",
			card: "Riddles in the Dark", foil: "0",
			wantSet: "PSPL", wantNumber: "12",
		},
		{
			desc: "a MagicFest Lightning Bolt names no promo type of its own, so the bare shelf tag is pinned by hand",
			jp:   "■テキストレス■《稲妻/Lightning Bolt》[MagicFest] 赤",
			en:   "■Textless■《Lightning Bolt》[MagicFest]",
			card: "Lightning Bolt", foil: "0",
			wantSet: "PF19", wantNumber: "1",
		},
		{
			desc: "a WPN promo filed under the set the card was drawn from",
			jp:   "【Foil】《メリー・ジェーン・ワトソン/Mary Jane Watson》(マジック・アカデミープロモ)[流星マーク] 白",
			en:   "【Foil】《Mary Jane Watson》[Magic Academy Promo]",
			card: "Mary Jane Watson", foil: "1",
			wantSet: "PW25", wantNumber: "11",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			b := withMagic(t)
			theCard, err := Preprocess(b, Product{
				ProductName: tt.jp, ProductNameEN: tt.en,
				CardName: tt.card, FoilFlag: tt.foil,
			})
			if err != nil {
				t.Fatalf("Preprocess(%q) = %v", tt.jp, err)
			}
			cardID, err := b.Match(theCard)
			if err != nil {
				t.Fatalf("Match(%q) = %v", theCard, err)
			}
			co, err := b.GetUUID(cardID)
			if err != nil {
				t.Fatal(err)
			}
			if co.SetCode != tt.wantSet || co.Number != tt.wantNumber {
				t.Errorf("Match(%q) = %s %s, want %s %s", theCard, co.SetCode, co.Number, tt.wantSet, tt.wantNumber)
			}
		})
	}
}

// TestBuylistPromoShelf pins the buylist's spellings of the same shelf,
// which names it by its shooting-star mark and the card by the program
// that handed it out, and the Pool Party drop's dazzle foil, told from the
// plain foil only by the marker in the foil's place. It also pins the
// shooting-star listings whose series names no shelf a per-card row
// already answers: the Spotlight Series and 30th Anniversary History
// promos, and four cards whose treatment and series were reaching the
// wrong printing untranslated together. A row with no set is one the
// buylist skips, having no printing to land on.
func TestBuylistPromoShelf(t *testing.T) {
	for _, tt := range []struct {
		title, wantSet, wantNumber string
	}{
		{"【EN】【Foil】《止められぬ斬鬼/Unstoppable Slasher》(ジャパンスタンダードカッププロモ)[流星マーク] 黒", "PJSC", "2026-2"},
		{"【EN】【Foil】■ボーダーレス■《ゾンビ使い/Zombie Master》(褒賞プロモ)[流星マーク] 黒", "PW24", "3"},
		{"【EN】【Foil】■拡張アート■《恐れを知らぬ者、カタラ/Katara, the Fearless》(MagicConプロモ)[流星マーク] 金", "PURL", "2025-3"},
		{"【EN】《紅蓮破/Pyroblast》[流星マーク] 赤", "PW23", "8"},
		{"【EN】【Pool Party・Foil】(SCD-288)《太陽の指輪/Sol Ring》[SLD] 茶R", "SLD", "IFIYW-10"},
		{"【EN】(SCD-288)《太陽の指輪/Sol Ring》[SLD] 茶R", "SLD", "IFIYW-5"},
		{"【EN】【Foil】(2062)■ボーダーレス■《Chancla relámpagos》//《稲妻のすね当て/Lightning Greaves》[SLD] 茶", "SLD", "2062★"},
		{"【EN】(2062)■ボーダーレス■《Chancla relámpagos》//《稲妻のすね当て/Lightning Greaves》[SLD] 茶", "SLD", "2062"},
		// Spotlight Series: no per-card row, resolved by treatment alone.
		{"【EN】【Foil】■ボーダーレス■《ミッドガルの傭兵、クラウド/Cloud, Midgar Mercenary》(スポットライトシリーズプロモ)[流星マーク] 白", "PSPL", "4"},
		{"【EN】【Foil】■拡張アート■《暗闇のなぞなぞ勝負/Riddles in the Dark》(スポットライトシリーズプロモ)[流星マーク] 青", "PSPL", "12"},
		{"【EN】■ボーダーレス■《失せろ/Get Lost》(スポットライトシリーズプロモ)[流星マーク] 白", "PSPL", "5"},
		{"【EN】《峰の恐怖/Terror of the Peaks》(スポットライトシリーズプロモ)[流星マーク] 赤", "PSPL", "1"},
		{"【EN】【Foil】《黒い太陽の日/Day of Black Sun》(スポットライトシリーズプロモ)[流星マーク] 黒", "PSPL", "7"},
		// 30th Anniversary History: the retro-framed and the plain printing.
		{"【EN】【Foil】■旧枠■《セラの天使/Serra Angel》(ヒストリープロモ)[流星マーク] 白", "P30H", "1★"},
		{"【EN】【Foil】《セラの天使/Serra Angel》(ヒストリープロモ)[流星マーク] 白", "P30H", "1"},
		// Four cards whose treatment and shelf, run together untranslated,
		// reached the wrong printing (FIN 38/45, JMP 25, SPM 208); Sethron
		// resolves via a new editionTable row, the rest via promoMap.
		{"【EN】【Foil】■ボーダーレス■《古代魔法「アルテマ」/Ultima》(スタンダード・ショーダウン)[流星マーク] 白", "PSS5", "1"},
		{"【EN】【Foil】《ハールーンの将軍、セスロン/Sethron, Hurloon General》(旧正月プロモ)[流星マーク] 赤", "PL21", "1★"},
		{"【EN】【Foil】■ボーダーレス■《ザックス・フェア/Zack Fair》(その他プロモ)[流星マーク] 白", "PMEI", "2026-3"},
		{"【EN】■拡張アート■《ピーター・パーカー/Peter Parker》/《アメイジング・スパイダーマン/Amazing Spider-Man》(その他プロモ)[流星マーク] 白R", "PMEI", "2025-22"},
		// Shooting-star rows the matcher widened to the set's own card
		// (FIN 243, ONE 26, SPM 157, TLA 54, TMT 109/112, HOB 29/32).
		{"【EN】【Foil】■ボーダーレス■《SeeDの傭兵、スコール/Squall, SeeD Mercenary》(スタンダード・ショーダウン)[流星マーク] 金", "PSS5", "2"},
		{"【EN】【Foil】■ボーダーレス■《骨化/Ossification》(スタンダード・ショーダウン)[流星マーク] 白", "PCBB", "1"},
		{"【EN】【Foil】《アルティメット・グリーンゴブリン/Ultimate Green Goblin》(マジック・プレゼンツプロモ)[流星マーク] 金", "PW25", "12"},
		{"【EN】【Foil】《ばあば/Gran-Gran》(コマンダーイベントプロモ)[流星マーク] 青", "PW25", "14"},
		{"【EN】【Foil】■ボーダーレス■《食料配達人/Courier of Comestibles》(コマンダーイベントプロモ)[流星マーク] 緑", "PW26", "6"},
		{"【EN】【Foil】■ボーダーレス■《ピリ辛オートミールピザ/Spicy Oatmeal Pizza》(マジック・プレゼンツプロモ)[流星マーク] 赤", "PW26", "7"},
		{"【EN】【Foil】■拡張アート■《思いがけない客たち/An Unexpected Party》(マジック・プレゼンツプロモ)[流星マーク] 白", "PW26", "14"},
		{"【EN】【Foil】■拡張アート■《幸運児、ビルボ/Bilbo, Luckwearer》(コマンダーイベントプロモ)[流星マーク] 青", "PW26", "15"},
		// The Marvel Legends compound entry named a set code as a bare
		// word, which only ever reaches the wrong printing as a variant;
		// spelling it out resolves the shelf's own set directly.
		{"【EN】【Foil】■ボーダーレス■《スタークによる改良、アイアン・スパイダー/Iron Spider, Stark Upgrade》(マーベル・レジェンドプロモ)[流星マーク] 茶", "LMAR", "4"},
		{"【EN】【Foil】■ボーダーレス■《恐ろしき癒し手、アンチヴェノム/Anti-Venom, Horrifying Healer》(マーベル・レジェンドプロモ)[流星マーク] 白", "LMAR", "1"},
		{"【EN】【Foil】■ボーダーレス■《スペクタキュラー・スパイダーマン/Spectacular Spider-Man》(マーベル・レジェンドプロモ)[流星マーク] 白", "LMAR", "2"},
		// A stamped copy lands where the set files a promo pack printing.
		{"【EN】(022)■プロモスタンプ付■《復活した精霊信者、ニッサ/Nissa, Resurgent Animist》[MAT] 緑R", "PMAT", "22p"},
		{"【EN】(049)■プロモスタンプ付■《再鍛されたレガシー、カーン/Karn, Legacy Reforged》[MAT] 茶R", "", ""},
		{"【EN】【Foil】《セラのアバター/Serra Avatar》[テストプリント]", "", ""},
		{"【EN】【Foil】■FFTCG■《アルフィノ/Alphinaud》(その他イベント記念系)[その他プロモ]", "", ""},
		{"【EN】《ルビーの大メダル/Ruby Medallion》[エラーカード] 茶R", "", ""},
		// Only the misprint wording is 67†a; the plain error card is 67†.
		{"【EN】【Foil】《ラクァタスのチャンピオン/Laquatus's Champion》(印刷ミス)[エラーカード] 黒R", "PTOR", "67†a"},
		{"【EN】【Foil】《ラクァタスのチャンピオン/Laquatus's Champion》[エラーカード] 黒R", "PTOR", "67†"},
	} {
		t.Run(tt.title, func(t *testing.T) {
			b := withMagic(t)
			theCard, err := preprocess(b, tt.title)
			if tt.wantSet == "" {
				if !errors.Is(err, matcher.ErrUnsupported) {
					t.Errorf("preprocess(%q) = %v, want ErrUnsupported", tt.title, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("preprocess(%q) = %v", tt.title, err)
			}
			cardID, err := b.Match(theCard)
			if err != nil {
				t.Fatalf("Match(%q) = %v", theCard, err)
			}
			co, err := b.GetUUID(cardID)
			if err != nil {
				t.Fatal(err)
			}
			if co.SetCode != tt.wantSet || co.Number != tt.wantNumber {
				t.Errorf("Match(%q) = %s %s, want %s %s", theCard, co.SetCode, co.Number, tt.wantSet, tt.wantNumber)
			}
		})
	}
}

// TestRetailEnglishLineWording pins the retail wording the English line
// alone carries: a Guild Kit basic's own number, an APAC land's own shelf
// index (artist plus name is still ambiguous for Ron Spears' two Swamps),
// a MagicFest basic's year, and a The List reprint's own number, read off
// the second half of the slash code hareruya prints for it.
func TestRetailEnglishLineWording(t *testing.T) {
	for _, tt := range []struct {
		desc, jp, en, card, foil string
		wantSet, wantNumber      string
	}{
		{
			desc: "a Guild Kit basic's number is only in the English line",
			jp:   "《森/Forest》[GK2-RG] 土地",
			en:   "《Forest》[GK2-RG](106)",
			card: "Forest", foil: "0",
			wantSet: "GK2", wantNumber: "106",
		},
		{
			desc: "an APAC land is told apart by the storefront's own shelf index",
			jp:   "(APAC3)《平地/Plains》(Illus.Rebecca Guay)[APACランド] 土地",
			en:   "APAC3  《Plains》  Illus.Rebecca Guay",
			card: "Plains", foil: "0",
			wantSet: "PALP", wantNumber: "14",
		},
		{
			desc: "a second APAC land, another shelf index and color",
			jp:   "(APAC1)《山/Mountain》(Illus.Heather Hudson)[APACランド] 土地",
			en:   "APAC1  《Mountain》 Illus.Heather Hudson",
			card: "Mountain", foil: "0",
			wantSet: "PALP", wantNumber: "3",
		},
		{
			desc: "a MagicFest basic's year is the only number it carries",
			jp:   "【Foil】《島/Island》(2019年版)[MagicFest] 土地",
			en:   "【Foil】《Island》2019ver [Magic Fest]",
			card: "Island", foil: "1",
			wantSet: "PF19", wantNumber: "3",
		},
		{
			desc: "a The List reprint's own number is past the slash",
			jp:   "(EvK/DDO-020)《ルーンの母/Mother of Runes》[PWシンボル付き再版] 白U",
			en:   "(EvK/DDO-020)《Mother of Runes》[MB1]",
			card: "Mother of Runes", foil: "0",
			wantSet: "PLST", wantNumber: "DDO-20",
		},
		{
			desc: "a foil basic whose number opens the English line",
			jp:   "【Foil】■フルアート■《平地/Plains》[UND] 土地",
			en:   "【Foil】(088)■Full-Art■《Plains》[UND]",
			card: "Plains", foil: "1",
			wantSet: "UND", wantNumber: "88",
		},
		{
			desc: "a Normal Border basic is the set's non-full-art printing",
			jp:   "(237)《島/Island》(Normal Border)[ZEN] 土地",
			en:   "《Island》(237)Normal Border[ZEN]",
			card: "Island", foil: "0",
			wantSet: "ZEN", wantNumber: "237a",
		},
		{
			desc: "a Portal starter without flavor text",
			jp:   "《アナコンダ/Anaconda》(FT無し)[POR-P] 緑U",
			en:   "《Anaconda》[Basic Set Promos](POR) Starter Ver.",
			card: "Anaconda", foil: "0",
			wantSet: "POR", wantNumber: "158†",
		},
		{
			desc: "a series group that names a plane, not the set",
			jp:   "《Amateur Auteur》(THEROS)[UST] 白C",
			en:   "《Amateur Auteur》(THEROS) [UST]",
			card: "Amateur Auteur", foil: "0",
			wantSet: "UST", wantNumber: "3c",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			b := withMagic(t)
			theCard, err := Preprocess(b, Product{
				ProductName: tt.jp, ProductNameEN: tt.en,
				CardName: tt.card, FoilFlag: tt.foil,
			})
			if err != nil {
				t.Fatalf("Preprocess(%q) = %v", tt.jp, err)
			}
			cardID, err := b.Match(theCard)
			if err != nil {
				t.Fatalf("Match(%q) = %v", theCard, err)
			}
			co, err := b.GetUUID(cardID)
			if err != nil {
				t.Fatal(err)
			}
			if co.SetCode != tt.wantSet || co.Number != tt.wantNumber {
				t.Errorf("Match(%q) = %s %s, want %s %s", theCard, co.SetCode, co.Number, tt.wantSet, tt.wantNumber)
			}
		})
	}
}
