---
title: The room
layout: module
num: MODULE 02_
lead: Labs, exercises, prompts, reveals.
---

---
title: Try it in the terminal
kicker: Lab_
decks: [workshop]
notes: Lab slides get a live terminal in the workspace folder (labs.folder in respawn.yaml).
---

Every command on a lab slide also goes to the terminal's history.

```cmd
ls -la
```

---
title: Pair up and compare
kicker: Exercise_
timer: 5 MIN
center: true
exercise: pair-up
catalog: true
---

::: steps
- num: '01'
  title: Pick a slide
  text: Any slide of this deck.
- num: '02'
  title: Rewrite it
  text: Same content, fewer words.
:::

---
title: Ask the model
kicker: Prompt_
center: true
---

::: prompt
Explain what a context window is, in two sentences, to a new developer.
:::

::: reveal label="What a good answer says"
**Everything the model reads at once**: instructions, files, chat. It is finite, so what you leave out matters as much as what you put in.
:::

---
title: What did we learn?
kicker: Recap_
center: true
generic: true
notes: Recap slides go to the exercise catalog (x), to run again whenever you want.
---

::: callout icon=lightbulb tone=secondary
Slides are code: review them, diff them, ship them.
:::
