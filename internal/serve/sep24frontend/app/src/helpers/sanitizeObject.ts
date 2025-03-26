type AnyObject = {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  [key: string]: any;
};

export const sanitizeObject = <T extends AnyObject>(
  obj: T,
  noEmptyObj = false
) => {
  return Object.keys(obj).reduce((res, param) => {
    const paramValue = obj[param];

    const emptyObj = noEmptyObj && isEmptyObject(paramValue);

    if (paramValue && !emptyObj) {
      return { ...res, [param]: paramValue };
    }

    return res;
  }, {} as T);
};

const isEmptyObject = (obj: AnyObject) => {
  return Object.keys(obj).length === 0;
};
