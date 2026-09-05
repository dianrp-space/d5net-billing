"use client";

import * as React from "react";
import { createPortal } from "react-dom";
import type {
  DragCancelEvent,
  DragEndEvent,
  DragOverEvent,
  DragStartEvent,
  DropAnimation,
  UniqueIdentifier,
} from "@dnd-kit/core";
import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  MeasuringStrategy,
  MouseSensor,
  TouchSensor,
  closestCorners,
  defaultDropAnimationSideEffects,
  useDroppable,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  defaultAnimateLayoutChanges,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
  type AnimateLayoutChanges,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { cn } from "@/lib/utils";

type KanbanContextValue<T> = {
  columns: Record<string, T[]>;
  setColumns: (columns: Record<string, T[]>) => void;
  getItemId: (item: T) => string;
  columnIds: string[];
  activeId: UniqueIdentifier | null;
  findContainer: (id: UniqueIdentifier) => string | undefined;
  isColumn: (id: UniqueIdentifier) => boolean;
};

const KanbanContext = React.createContext<KanbanContextValue<unknown> | null>(null);
const IsOverlayContext = React.createContext(false);
const ItemListenersContext = React.createContext<ReturnType<typeof useSortable>["listeners"] | undefined>(undefined);

function useKanban<T>() {
  const ctx = React.useContext(KanbanContext);
  if (!ctx) throw new Error("Kanban components must be used within <Kanban>");
  return ctx as KanbanContextValue<T>;
}

const animateLayoutChanges: AnimateLayoutChanges = (args) =>
  defaultAnimateLayoutChanges({ ...args, wasDragging: true });

const dropAnimationConfig: DropAnimation = {
  sideEffects: defaultDropAnimationSideEffects({
    styles: { active: { opacity: "0.4" } },
  }),
};

export type KanbanMoveEvent = {
  event: DragEndEvent;
  activeContainer: string;
  activeIndex: number;
  overContainer: string;
  overIndex: number;
};

export type KanbanCommitMeta<T> = {
  kind: "item";
  event: DragEndEvent;
  activeContainer: string;
  activeIndex: number;
  overContainer: string;
  overIndex: number;
  previousValue: Record<string, T[]>;
};

export type KanbanProps<T> = {
  value: Record<string, T[]>;
  onValueChange: (value: Record<string, T[]>) => void;
  getItemValue: (item: T) => string;
  children: React.ReactNode;
  className?: string;
  onMove?: (event: KanbanMoveEvent) => void;
  onValueCommit?: (value: Record<string, T[]>, meta: KanbanCommitMeta<T>) => void;
  restoreOnCancel?: boolean;
};

export function Kanban<T>({
  value,
  onValueChange,
  getItemValue,
  children,
  className,
  onMove,
  onValueCommit,
  restoreOnCancel = true,
}: KanbanProps<T>) {
  const columns = value;
  const setColumns = onValueChange;
  const [activeId, setActiveId] = React.useState<UniqueIdentifier | null>(null);

  const valueRef = React.useRef(value);
  const getItemValueRef = React.useRef(getItemValue);
  React.useLayoutEffect(() => {
    valueRef.current = value;
    getItemValueRef.current = getItemValue;
  });

  const dragOriginRef = React.useRef<{
    value: Record<string, T[]>;
    container: string | undefined;
    index: number;
  } | null>(null);

  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 200, tolerance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const columnIds = React.useMemo(() => Object.keys(columns), [columns]);

  const isColumn = React.useCallback(
    (id: UniqueIdentifier) => columnIds.includes(String(id)),
    [columnIds],
  );

  const findContainer = React.useCallback(
    (id: UniqueIdentifier) => {
      if (isColumn(id)) return String(id);
      return columnIds.find((key) => columns[key]?.some((item) => getItemValue(item) === id));
    },
    [columns, columnIds, getItemValue, isColumn],
  );

  const commitChange = React.useCallback(
    (finalValue: Record<string, T[]>, event: DragEndEvent) => {
      if (!onValueCommit) return;
      const origin = dragOriginRef.current;
      if (!origin?.container) return;
      const id = event.active.id;
      const getId = getItemValueRef.current;
      let overContainer: string | undefined;
      let overIndex = -1;
      for (const key of Object.keys(finalValue)) {
        const found = finalValue[key].findIndex((item) => getId(item) === id);
        if (found !== -1) {
          overContainer = key;
          overIndex = found;
          break;
        }
      }
      if (overContainer === undefined) return;
      if (overContainer === origin.container && overIndex === origin.index) return;
      onValueCommit(finalValue, {
        kind: "item",
        event,
        activeContainer: origin.container,
        activeIndex: origin.index,
        overContainer,
        overIndex,
        previousValue: origin.value,
      });
    },
    [onValueCommit],
  );

  const handleDragStart = React.useCallback(
    (event: DragStartEvent) => {
      setActiveId(event.active.id);
      const snapshot = valueRef.current;
      const id = event.active.id;
      const getId = getItemValueRef.current;
      let container: string | undefined;
      let index = -1;
      for (const key of Object.keys(snapshot)) {
        const found = snapshot[key].findIndex((item) => getId(item) === id);
        if (found !== -1) {
          container = key;
          index = found;
          break;
        }
      }
      dragOriginRef.current = {
        value: Object.fromEntries(Object.entries(snapshot).map(([k, v]) => [k, [...v]])),
        container,
        index,
      };
    },
    [],
  );

  const handleDragOver = React.useCallback(
    (event: DragOverEvent) => {
      if (onMove) return;
      const { active, over } = event;
      if (!over) return;
      if (isColumn(active.id)) return;

      const activeContainer = findContainer(active.id);
      const overContainer = findContainer(over.id);
      if (!activeContainer || !overContainer) return;

      if (activeContainer !== overContainer) {
        const activeItems = columns[activeContainer];
        const overItems = columns[overContainer];
        const activeIndex = activeItems.findIndex((item) => getItemValue(item) === active.id);
        let overIndex = overItems.findIndex((item) => getItemValue(item) === over.id);
        if (isColumn(over.id)) overIndex = overItems.length;
        if (activeIndex < 0) return;

        const newActiveItems = [...activeItems];
        const newOverItems = [...overItems];
        const [movedItem] = newActiveItems.splice(activeIndex, 1);
        newOverItems.splice(overIndex < 0 ? newOverItems.length : overIndex, 0, movedItem);
        setColumns({
          ...columns,
          [activeContainer]: newActiveItems,
          [overContainer]: newOverItems,
        });
      } else {
        const activeIndex = columns[activeContainer].findIndex((item) => getItemValue(item) === active.id);
        const overIndex = columns[activeContainer].findIndex((item) => getItemValue(item) === over.id);
        if (activeIndex >= 0 && overIndex >= 0 && activeIndex !== overIndex) {
          setColumns({
            ...columns,
            [activeContainer]: arrayMove(columns[activeContainer], activeIndex, overIndex),
          });
        }
      }
    },
    [columns, findContainer, getItemValue, isColumn, onMove, setColumns],
  );

  const handleDragCancel = React.useCallback(
    (event: DragCancelEvent) => {
      const origin = dragOriginRef.current;
      if (restoreOnCancel && origin && !onMove) {
        setColumns(origin.value);
      } else if (onValueCommit && origin && !onMove) {
        commitChange(valueRef.current, event);
      }
      dragOriginRef.current = null;
      setActiveId(null);
    },
    [commitChange, onMove, onValueCommit, restoreOnCancel, setColumns],
  );

  const handleDragEnd = React.useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      setActiveId(null);

      if (onMove && !isColumn(active.id)) {
        const activeContainer = findContainer(active.id);
        const overContainer = over ? findContainer(over.id) : undefined;
        if (activeContainer && overContainer) {
          const activeIndex = columns[activeContainer].findIndex((item) => getItemValue(item) === active.id);
          const overIndex = isColumn(over!.id)
            ? columns[overContainer].length
            : columns[overContainer].findIndex((item) => getItemValue(item) === over!.id);
          onMove({ event, activeContainer, activeIndex, overContainer, overIndex });
        }
        dragOriginRef.current = null;
        return;
      }

      commitChange(valueRef.current, event);
      dragOriginRef.current = null;
    },
    [columns, commitChange, findContainer, getItemValue, isColumn, onMove],
  );

  const contextValue = React.useMemo(
    () => ({
      columns,
      setColumns,
      getItemId: getItemValue,
      columnIds,
      activeId,
      findContainer,
      isColumn,
    }),
    [activeId, columnIds, columns, findContainer, getItemValue, isColumn, setColumns],
  );

  return (
    <KanbanContext.Provider value={contextValue as KanbanContextValue<unknown>}>
      <DndContext
        sensors={sensors}
        collisionDetection={closestCorners}
        measuring={{ droppable: { strategy: MeasuringStrategy.Always } }}
        onDragStart={handleDragStart}
        onDragOver={handleDragOver}
        onDragEnd={handleDragEnd}
        onDragCancel={handleDragCancel}
      >
        <div
          data-slot="kanban"
          data-dragging={activeId !== null || undefined}
          className={cn(activeId !== null && "cursor-grabbing", className)}
        >
          {children}
        </div>
      </DndContext>
    </KanbanContext.Provider>
  );
}

export function KanbanBoard({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="kanban-board"
      className={cn("flex gap-3 overflow-x-auto pb-2", className)}
      {...props}
    />
  );
}

export function KanbanColumn({
  value,
  className,
  disabled,
  children,
  ...props
}: React.ComponentProps<"div"> & { value: string; disabled?: boolean }) {
  const isOverlay = React.useContext(IsOverlayContext);
  const { columns, getItemId } = useKanban();
  const items = columns[value] ?? [];
  const itemIds = items.map(getItemId);

  const { setNodeRef, isOver } = useDroppable({
    id: value,
    disabled: disabled || isOverlay,
    data: { type: "column" },
  });

  return (
    <div
      ref={isOverlay ? undefined : setNodeRef}
      data-slot="kanban-column"
      data-value={value}
      data-over={isOver || undefined}
      className={cn(
        "group/kanban-column flex w-[260px] shrink-0 flex-col rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--panel-muted)]/40",
        isOver && "border-[var(--accent)] ring-2 ring-[var(--focus-ring)]",
        className,
      )}
      {...props}
    >
      <SortableContext items={itemIds} strategy={verticalListSortingStrategy}>
        {children}
      </SortableContext>
    </div>
  );
}

export function KanbanColumnHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="kanban-column-header"
      className={cn("flex items-center justify-between gap-2 border-b border-[var(--border)] px-3 py-2.5", className)}
      {...props}
    />
  );
}

export function KanbanColumnContent({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="kanban-column-content"
      className={cn("flex max-h-[min(70vh,720px)] flex-col gap-2 overflow-y-auto p-2", className)}
      {...props}
    />
  );
}

export function KanbanItem({
  value,
  className,
  disabled,
  children,
  ...props
}: React.ComponentProps<"div"> & { value: string; disabled?: boolean }) {
  const isOverlay = React.useContext(IsOverlayContext);
  const {
    setNodeRef,
    transform,
    transition,
    attributes,
    listeners,
    isDragging,
  } = useSortable({
    id: value,
    disabled: disabled || isOverlay,
    animateLayoutChanges,
  });

  const style: React.CSSProperties = {
    transition,
    transform: CSS.Transform.toString(transform),
  };

  const composedClassName = cn(
    !disabled && !isOverlay && "cursor-grab touch-none active:cursor-grabbing",
    !isOverlay && isDragging && "opacity-30",
    disabled && "opacity-50",
    className,
  );

  return (
    <ItemListenersContext.Provider value={isOverlay ? undefined : listeners}>
      <div
        ref={isOverlay ? undefined : setNodeRef}
        style={isOverlay ? undefined : style}
        data-slot="kanban-item"
        data-value={value}
        data-dragging={isDragging || undefined}
        className={composedClassName}
        {...(isOverlay ? {} : attributes)}
        {...(isOverlay || disabled ? {} : listeners)}
        {...props}
      >
        {children}
      </div>
    </ItemListenersContext.Provider>
  );
}

export function KanbanItemHandle({ className, ...props }: React.ComponentProps<"button">) {
  const listeners = React.useContext(ItemListenersContext);
  return (
    <button
      type="button"
      data-slot="kanban-item-handle"
      className={cn("cursor-grab touch-none active:cursor-grabbing", className)}
      {...listeners}
      {...props}
    />
  );
}

export function KanbanOverlay({
  children,
  className,
  ...props
}: Omit<React.ComponentProps<typeof DragOverlay>, "children"> & {
  children?:
    | React.ReactNode
    | ((params: { value: UniqueIdentifier; variant: "column" | "item" }) => React.ReactNode);
}) {
  const { activeId, isColumn } = useKanban();
  const [mounted, setMounted] = React.useState(false);
  React.useEffect(() => setMounted(true), []);

  const variant = activeId && isColumn(activeId) ? "column" : "item";
  const content =
    activeId && children
      ? typeof children === "function"
        ? children({ value: activeId, variant })
        : children
      : null;

  if (!mounted) return null;

  return createPortal(
    <DragOverlay dropAnimation={dropAnimationConfig} className={cn("z-50", className)} {...props}>
      <IsOverlayContext.Provider value={true}>{content}</IsOverlayContext.Provider>
    </DragOverlay>,
    document.body,
  );
}
