# Visual task app (Orbit)

An interactive wireframe for a visual personal task manager. Topics tile together like a
disk-space treemap (in the style of WizTree): each topic's area is its share of open work,
and each small cell inside it is one task.

Status: concept wireframe. Everything is in a single `index.html` with no build step or
dependencies. Data is saved only in your browser's `localStorage`.

## Run it

Open `index.html` in a browser.

## What it does

- **Topics and groups.** Drag a topic's header onto another topic to group them; drop it on the
  top bar to take it out of its group. Double-click a name to rename it.
- **Quick add.** Start typing anywhere to add a task: `Call the bank @ Admin`. After `@`, pick a
  topic from the suggestions (arrow keys and Tab); an unknown name creates a new topic.
- **Finished tasks become stars.** Ticking a task turns it into a star in the topic's header and
  frees its space on the map. Click the stars to reopen or clear finished tasks.
- **Colours.** Click any colour dot to recolour a topic or a whole group.
- **Navigation.** Drag or scroll to pan; Cmd + scroll (Ctrl on Windows) or pinch to zoom.
  Double-click a tile to zoom into it.

## Open questions

- Should size come from task count, estimates, or due dates?
- Colour by topic, by group, or by urgency?
- Sync tasks with monday.com, or keep them local?
