const leaves = new WeakMap<Element, (done: () => void) => void>();
export function registerDialogLeave(
  element: Element,
  leave: (done: () => void) => void,
) {
  leaves.set(element, leave);
}
export function leaveDialog(element: Element, done: () => void) {
  const leave = leaves.get(element);
  if (leave)
    leave(() => {
      leaves.delete(element);
      done();
    });
  else done();
}
