# Add Toolbar Button

Add a new button to the viewer toolbar in web/public/index.html.

## Button to add: $ARGUMENTS

## Steps

1. Add a `<button>` element inside the `.viewer-toolbar` div using the `.toolbar-btn` class
2. Give it a descriptive `title` attribute (shows as tooltip on hover)
3. Add an `onclick` handler wired to a new JS function
4. Implement the function in the `<script>` block
5. If the button has an active/toggle state, use the `.active` class pattern already established

Keep the button icon as a single unicode character or emoji. Keep the implementation self-contained in index.html.
