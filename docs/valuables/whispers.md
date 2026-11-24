# Whispers and haggling (D32)

## Prices

Per valuable a round has an **ask** and a **floor**:

- first ask `ref × min(MaxAsk, 100 + AskPerRarity × R) / 100`, `MaxAsk` 150, `AskPerRarity` 12.5 (`R
  = 4` asks 150 %), plus the enchant scrolls at their reference price;
- floor `max(W × FloorPct / 100, storePrice(ref))`, `FloorPct` 100, `W` the
  [worth](score.md#rarity-marks-and-worth): never below the store's ask, the reference price or the
  scrolls spent. `storePrice` prices the bare template (`store.go:446-456`), so a +5 Conjuror's
  Staff (scroll 955 at 50000) has the floor 644000 + 5 × 50000 = 894000; an item with no grade
  (Sword of Watershadow) has its reference price as floor at any `Enchant`;
- each unsold round lowers the ask by `RoundStep` (10 points of the reference price), never below
  the floor.

## Naming an item

Case-insensitive, over the entries with carried units not released. A word of an entry's name
**points at** it when it has at least 4 letters, is no stop word (`of`, `the`, `and`, `for`, `with`,
`from`) and matches a whole word of the line. A pointing word is **distinctive** unless it is a
**kind word** (`scroll`, `enchant`, `recipe`, `grade`, `weapon`, `armor`). Tested in order, a line:

1. **names** an entry whose full name it contains, the longest winning (`sword of watershadow`);
2. else, if its distinctive words point at any entry, names it when they point at exactly one, and
   **points at several** when at two or more;
3. else does the same with its kind words;
4. else **names nothing**.

## Reading a price

A price is a whole word of the line, once a contained full item name is taken out: digits (an
optional `.` or `,` decimal part) with `k`, `kk` or `m`, or no suffix and at least 1000 (`150000`,
`150k`, `1.2kk`, `1.2m`; `150 000` is no price). Never a price: a number after `+` (`+5`), with `x`
right before or after (`2x`, `x2`), inside a word or among other signs (`Halberd(60%)`,
`items/5400-5499.xml:476`), a bare number under 1000. A price is per unit, as the labels show. A
line with two or more prices takes none and gets the asks. Examples: [tests](#tests).

## Replies in a round

A whisper in a round (no bot marker, not a command) goes to the responder, not the canned reply
(`chatHeard`):

- with no price, naming a held item or asking a price (`?`, `how much`, `price`): `<name> <current>,
  trade me`, the partner's current price (the ask until countered); pointing at several: `WTS <name>
  <ask>, <name> <ask>, which?`;
- with a price: an **offer** on the item it names; naming nothing, on the only valuable, else the
  top-scored one; pointing at several, no offer (the asks as above). An offer under half the floor
  is answered like a question and uses no counter. Otherwise:
  - at or above the current price: agreed at the current price, `ok <price>, trade me`;
  - below it with `Counters` left (2 per partner and item a round): the counter `max(floor, (offer +
    current) / 2)` rounded up to 1k, at most `current`, `<price> lowest`, the new current price;
  - no counters left: agreed at the offer when at or above the floor, else `sorry, no`;
- anything else: `WTS <names>, trade me`.

**Form.** A reply starts with a lowercase word, an item name (`+` if enchanted) or `WTS`, never an
upper-case walker command, which the server drops (case-sensitive prefix,
`handler/chat/chat.go:78-84,114-121`); one that would gets a lowercase `wts`/`ok` lead. It is at
most 100 UTF-16 units (`MaxTextLength`, `handler/chat/chat.go:63-64`; `Admit` drops longer lines,
`:99-101`; `chatMaxText`, `craftorder.go:23`), since a dropped reply still counts in the caps: a
listing reply (`which?`, `WTS <names>, trade me`, `WTS <names> again soon`) takes whole labels in
score order as they fit (six entries with names such as `Recipe: Halberd(60%)` or `Conjuror's Staff
(D)` do not), and a one-item label too long alone is cut on a rune by `clipChat`
(`craftorder.go:231-240`), which no shipped name needs. Composed when it goes out.

## Pacing, queue and caps

- **Delay:** `DelayMin`-`DelayMax` (3-12 s, the canned `ChatPolicy` delay, `chat.go:32-40`) after
  the whisper, then the chat gate (3 s).
- **Queue:** today's path holds one reply and drops a whisper heard meanwhile (`chat.go:128`,
  `b.chat.reply.kind != actHold`). The responder queues one reply per partner (a newer whisper
  replaces it), any number of partners, sent earliest due first by rule "chat" (`rules.go:482`). The
  queue outlives the round for `AnswerAfter`: a reply waiting at the round's end goes out as an
  after-round reply, one waiting at the next round's start is composed on it, one waiting past
  `AnswerAfter` is dropped; the session's end clears it.
- **Caps:** `RepliesPerPartner` (4) and `Replies` (20) per round, then silence: about 40 an hour.
  They are outside the canned `chatLog` and its `PerSender` (10 min) and `Total` (5 an hour) caps
  (`chat.go:32-40`), which would cut a haggle after one line; D32 names the amendment.

## After a round

For `AnswerAfter` (30 min) after a round's end, until the next starts, a whisper (no marker, no
command) naming a held entry (over every entry, carried or stored) or asking a price goes to the
responder, with its delay, queue and the ended round's caps, instead of a random canned `lol`, `brb`
or `hi` (`defaultChatPolicy`, `chat.go:32-40`; `chatHeard`, `chat.go:121-136`):

- an entry not released: `<name> not now, wts again soon`;
- a released entry listed in the open store: `<name> in my store <price>` (V7c);
- a released entry not listed (stored for the release withdrawal, carried before the opening or in a
  full town, or left out by the budget): `<name> not now, wts again soon` (V7c);
- a price question naming nothing: `WTS <names> again soon`.

No offer or trade outside a round: the bot may be in its store, where the server refuses trades
(`network/trade.go:69-75`). Other whispers, and all past `AnswerAfter` or before any round, get the
canned reply with today's caps.

## Tests

`TestValuablesWhisperHaggle` (V5, a round opened with `openRound`):

- **Haggle:** a question gets the ask; an offer below it a counter, a second below the counter the
  last counter, a third at the floor is agreed, one below refused; a +5 valuable's floor includes
  five scrolls and its bare reference price is refused after the counters; a marked name, a
  partner's fifth reply and the 21st get nothing; replies go 3-12 s after their whisper; two
  partners 1 s apart both get one, and a partner's second whisper replaces its pending reply; no
  reply counts in `chatLog`; no canned reply in a round.
- **Names:** with Sword of Watershadow and Tears of Eva (both names hold `of`,
  `items/0100-0199.xml:300`, `items/0300-0399.xml:224`), "of course" and "out of adena" name nothing
  (no price: `WTS <names>, trade me`; with one the offer goes to the top-scored), "tears?" names
  Tears of Eva, "watershadow 150k" offers on the sword, "sword of watershadow" names it; with two
  scrolls "scroll 50k" points at both, takes no offer, gets both asks; with one scroll and the sword
  it offers on the scroll, and "watershadow scroll" names the sword.
- **Prices:** "how much for +5 conjuror's staff?" gets the ask, no counter used; "2x tears 150k"
  offers 150000 a unit; with a fixture entry `Recipe: Halberd(60%)`, "recipe: halberd(60%) 300k"
  offers 300000; "tears 500" and "tears 150k 140k" no offer; under half the floor: the current price, both counters left.
- **Length:** with seven long-named entries, a question pointing at several, a line naming none and
  an after-round question each get at most 100 UTF-16 units of whole top-scored labels; each reply
  counts once.
- **After the round** (`endRound`, still carried; stored is V7a's
  [`TestValuablesRoundDeposit`](round-end.md#tests)): within `AnswerAfter` a whisper naming it gets
  `<name> not now, wts again soon` 3-12 s later under the round's caps, no offer; a reply pending at
  `endRound` is the after-round one; an unrelated whisper gets the canned reply; past `AnswerAfter`
  all do.
