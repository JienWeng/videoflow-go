# Official current shadcn-svelte primitive provenance
Fetched 2026-11-12 from the www.shadcn-svelte.com official nova registry. No demos or scripted AI transports included.
Registry contents preserved except normal CLI substitution of $UTILS$, $UI$, $LIB$, $HOOKS$ and IconPlaceholder to selected Lucide components. Existing primitives not overwritten.
Transitive registry libs placed at src/lib/highlighter.ts and src/lib/hooks/use-highlighted-code.svelte.ts.
NPM: added @shadcn-svelte/primitives ^0.0.1 and shiki ^3.20.0; raised registry-required bits-ui ^2.20.0 and tailwind-variants ^3.3.0.
No ai-sdk.ts helper needed: createChat helper in examples scripts demo conversations, custom Go transport stays intact.

## attachment
https://www.shadcn-svelte.com/registry/styles/nova/attachment.json
```ts
import Action from "./attachment-action.svelte";
import Actions from "./attachment-actions.svelte";
import Content from "./attachment-content.svelte";
import Description from "./attachment-description.svelte";
import Group from "./attachment-group.svelte";
import Media from "./attachment-media.svelte";
import Title from "./attachment-title.svelte";
import Trigger from "./attachment-trigger.svelte";
import Root, {
	attachmentVariants,
	type AttachmentOrientation,
	type AttachmentSize,
	type AttachmentState,
} from "./attachment.svelte";

export {
	Root,
	Group,
	Media,
	Content,
	Title,
	Description,
	Actions,
	Action,
	Trigger,
	attachmentVariants,
	type AttachmentSize,
	type AttachmentOrientation,
	type AttachmentState,
	//
	Root as Attachment,
	Group as AttachmentGroup,
	Media as AttachmentMedia,
	Content as AttachmentContent,
	Title as AttachmentTitle,
	Description as AttachmentDescription,
	Actions as AttachmentActions,
	Action as AttachmentAction,
	Trigger as AttachmentTrigger,
};

```
## bubble
https://www.shadcn-svelte.com/registry/styles/nova/bubble.json
```ts
import Content from "./bubble-content.svelte";
import Group from "./bubble-group.svelte";
import Reactions, {
	bubbleReactionsVariants,
	type BubbleReactionsAlign,
	type BubbleReactionsSide,
} from "./bubble-reactions.svelte";
import Root, { bubbleVariants, type BubbleVariant } from "./bubble.svelte";

export {
	Root,
	Group,
	Content,
	Reactions,
	bubbleVariants,
	bubbleReactionsVariants,
	type BubbleVariant,
	type BubbleReactionsSide,
	type BubbleReactionsAlign,
	//
	Root as Bubble,
	Group as BubbleGroup,
	Content as BubbleContent,
	Reactions as BubbleReactions,
};

```
## code-block
https://www.shadcn-svelte.com/registry/styles/nova/code-block.json
```ts
import Actions from "./code-block-actions.svelte";
import Content from "./code-block-content.svelte";
import CopyButton from "./code-block-copy-button.svelte";
import Footer from "./code-block-footer.svelte";
import Header from "./code-block-header.svelte";
import Title from "./code-block-title.svelte";
import Line, {
	codeBlockLineVariants,
	type CodeBlockLineToken,
	type CodeBlockLineVariant,
} from "./code-block-line.svelte";
import Root, {
	codeBlockVariants,
	type CodeBlockSize,
	type CodeBlockVariant,
} from "./code-block.svelte";

export {
	Root,
	Header,
	Title,
	Actions,
	CopyButton,
	Content,
	Line,
	Footer,
	codeBlockVariants,
	codeBlockLineVariants,
	type CodeBlockVariant,
	type CodeBlockSize,
	type CodeBlockLineVariant,
	type CodeBlockLineToken,
	//
	Root as CodeBlock,
	Header as CodeBlockHeader,
	Title as CodeBlockTitle,
	Actions as CodeBlockActions,
	CopyButton as CodeBlockCopyButton,
	Content as CodeBlockContent,
	Line as CodeBlockLine,
	Footer as CodeBlockFooter,
};

```
## empty
https://www.shadcn-svelte.com/registry/styles/nova/empty.json
```ts
import Content from "./empty-content.svelte";
import Description from "./empty-description.svelte";
import Header from "./empty-header.svelte";
import Media from "./empty-media.svelte";
import Title from "./empty-title.svelte";
import Root from "./empty.svelte";

export {
	Root,
	Header,
	Media,
	Title,
	Description,
	Content,
	//
	Root as Empty,
	Header as EmptyHeader,
	Media as EmptyMedia,
	Title as EmptyTitle,
	Description as EmptyDescription,
	Content as EmptyContent,
};

```
## hover-card
https://www.shadcn-svelte.com/registry/styles/nova/hover-card.json
```ts
import Content from "./hover-card-content.svelte";
import Portal from "./hover-card-portal.svelte";
import Trigger from "./hover-card-trigger.svelte";
import Root from "./hover-card.svelte";

export {
	Root,
	Content,
	Trigger,
	Portal,
	Root as HoverCard,
	Content as HoverCardContent,
	Trigger as HoverCardTrigger,
	Portal as HoverCardPortal,
};

```
## input-group
https://www.shadcn-svelte.com/registry/styles/nova/input-group.json
```ts
import Addon from "./input-group-addon.svelte";
import Button from "./input-group-button.svelte";
import Input from "./input-group-input.svelte";
import Text from "./input-group-text.svelte";
import Textarea from "./input-group-textarea.svelte";
import Root from "./input-group.svelte";

export {
	Root,
	Addon,
	Button,
	Input,
	Text,
	Textarea,
	//
	Root as InputGroup,
	Addon as InputGroupAddon,
	Button as InputGroupButton,
	Input as InputGroupInput,
	Text as InputGroupText,
	Textarea as InputGroupTextarea,
};

```
## item
https://www.shadcn-svelte.com/registry/styles/nova/item.json
```ts
import Actions from "./item-actions.svelte";
import Content from "./item-content.svelte";
import Description from "./item-description.svelte";
import Footer from "./item-footer.svelte";
import Group from "./item-group.svelte";
import Header from "./item-header.svelte";
import Media from "./item-media.svelte";
import Separator from "./item-separator.svelte";
import Title from "./item-title.svelte";
import Root from "./item.svelte";

export {
	Root,
	Group,
	Separator,
	Header,
	Footer,
	Content,
	Title,
	Description,
	Actions,
	Media,
	//
	Root as Item,
	Group as ItemGroup,
	Separator as ItemSeparator,
	Header as ItemHeader,
	Footer as ItemFooter,
	Content as ItemContent,
	Title as ItemTitle,
	Description as ItemDescription,
	Actions as ItemActions,
	Media as ItemMedia,
};

```
## marker
https://www.shadcn-svelte.com/registry/styles/nova/marker.json
```ts
import Content from "./marker-content.svelte";
import Icon from "./marker-icon.svelte";
import Root, { markerVariants, type MarkerVariant } from "./marker.svelte";

export {
	Root,
	Icon,
	Content,
	markerVariants,
	type MarkerVariant,
	//
	Root as Marker,
	Icon as MarkerIcon,
	Content as MarkerContent,
};

```
## message
https://www.shadcn-svelte.com/registry/styles/nova/message.json
```ts
import Avatar from "./message-avatar.svelte";
import Content from "./message-content.svelte";
import Footer from "./message-footer.svelte";
import Group from "./message-group.svelte";
import Header from "./message-header.svelte";
import Root from "./message.svelte";

export {
	Root,
	Group,
	Avatar,
	Content,
	Header,
	Footer,
	//
	Root as Message,
	Group as MessageGroup,
	Avatar as MessageAvatar,
	Content as MessageContent,
	Header as MessageHeader,
	Footer as MessageFooter,
};

```
## message-scroller
https://www.shadcn-svelte.com/registry/styles/nova/message-scroller.json
```ts
import Button from "./message-scroller-button.svelte";
import Content from "./message-scroller-content.svelte";
import Item from "./message-scroller-item.svelte";
import Provider from "./message-scroller-provider.svelte";
import Viewport from "./message-scroller-viewport.svelte";
import Root from "./message-scroller.svelte";

export {
	useMessageScroller,
	useMessageScrollerScrollable,
	useMessageScrollerVisibility,
} from "@shadcn-svelte/primitives/message-scroller";

export {
	Provider,
	Root,
	Viewport,
	Content,
	Item,
	Button,
	//
	Provider as MessageScrollerProvider,
	Root as MessageScroller,
	Viewport as MessageScrollerViewport,
	Content as MessageScrollerContent,
	Item as MessageScrollerItem,
	Button as MessageScrollerButton,
};

```
## progress
https://www.shadcn-svelte.com/registry/styles/nova/progress.json
```ts
import Root from "./progress.svelte";

export {
	Root,
	//
	Root as Progress,
};

```
## trace
https://www.shadcn-svelte.com/registry/styles/nova/trace.json
```ts
import CollapsibleContent from "./trace-collapsible-content.svelte";
import CollapsibleTrigger from "./trace-collapsible-trigger.svelte";
import Collapsible from "./trace-collapsible.svelte";
import Description from "./trace-description.svelte";
import Icon from "./trace-icon.svelte";
import Title from "./trace-title.svelte";
import Root from "./trace.svelte";
import Line, { type TraceLineStatus } from "./trace-line.svelte";

export {
	Root,
	Line,
	Icon,
	Title,
	Description,
	Collapsible,
	CollapsibleTrigger,
	CollapsibleContent,
	type TraceLineStatus,
	//
	Root as Trace,
	Line as TraceLine,
	Icon as TraceIcon,
	Title as TraceTitle,
	Description as TraceDescription,
	Collapsible as TraceCollapsible,
	CollapsibleTrigger as TraceCollapsibleTrigger,
	CollapsibleContent as TraceCollapsibleContent,
};

```
## use-highlighted-code
https://www.shadcn-svelte.com/registry/styles/nova/use-highlighted-code.json
## highlighter
https://www.shadcn-svelte.com/registry/styles/nova/highlighter.json
Validation: npm run check (2026-11-12) passed with 0 errors and 0 warnings after registry alias/icon transformation. Existing Button already supports xs/icon-xs/icon-sm; no wrapper or Button compatibility deviation needed.
