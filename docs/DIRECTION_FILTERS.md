# Picking nodes for a Direction

A **Direction** is a named routing target your rules point at. Which nodes end
up inside it is decided by one field: the **node filter**.

The filter is a regular expression matched against the node's **final tag** —
exactly the text you see in the node list, after any prefix the subscription
adds. Leave it empty and the Direction takes every node.

## What you type

Type the **body** of the expression only. No slashes, no flags:

| You want | You type |
|---|---|
| German nodes, tagged `🇩🇪 Frankfurt` | `🇩🇪` |
| German **or** Dutch | `🇩🇪\|🇳🇱` |
| Tags starting with `DE-` | `^DE-` |
| Tags ending in `-premium` | `-premium$` |
| Everything **except** Russia | `🇷🇺` + tick **Invert** |

**Case never matters.** `de-` matches `DE-Frankfurt`. This is not a setting:
subscription tags arrive in whatever case the provider felt like, and matching
on case would break the moment they change it.

**Emoji work.** Country flags are the most reliable thing in a subscription
tag — providers rename cities, rarely flags.

## The pieces you'll actually use

| Piece | Means | Example |
|---|---|---|
| plain text | contains this text | `Frankfurt` |
| `\|` | either side | `🇩🇪\|🇳🇱\|🇧🇪` |
| `^` | start of the tag | `^AL:` |
| `$` | end of the tag | `2$` |
| `.` | any one character | `DE-.` |
| `.*` | any text, including none | `DE.*prem` |
| `[abc]` | one of these characters | `[0-9]` |
| `\.` | a literal dot | `example\.com` |

Everything else follows [Go's RE2 syntax](https://github.com/google/re2/wiki/Syntax);
look-behind and backreferences are the notable omissions.

## Invert

Ticking **Invert** keeps the nodes that do **not** match. It is the honest way
to say "everything but": writing a regex that excludes something is a puzzle,
ticking a box is not.

Invert does nothing when the filter is empty — "everything except nothing" is
still everything.

## Default node

The **default node** field is a second regular expression, matched the same
way. The first node that matches becomes the Direction's starting choice. Leave
it empty and the core picks the first node in the list.

If the Direction also has auto-select on, its `<tag>-auto` group becomes the
default instead — unless this field matched something, in which case your
explicit choice wins.

## When nothing matches

A filter that matches no nodes leaves the Direction empty, and an empty
Direction **blocks** the traffic of the rules pointing at it (with a direct
option available to switch to by hand). This is deliberate: silently sending
that traffic outside the VPN would be worse. The launcher warns you when this
happens and names the Direction.

A **broken** expression — an unclosed bracket, a stray `(` — is ignored
entirely, as if the field were empty, so a typo never costs you every node.
