export function publicationDestination(frozen, input) {
  if (!frozen?.attempted) return input;
  const full = frozen.ticket?.repository;
  if (typeof full !== 'string' || full.split('/').length !== 2) throw new Error('Не найден исходный репозиторий. Сохрани план и проверь результат в GitHub.');
  return full.split('/')[1];
}
export function matchingReceipt(receipt, frozen) { return !!receipt && !!frozen && receipt.planId === frozen.plan.id; }
